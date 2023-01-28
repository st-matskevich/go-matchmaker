package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/docker/docker/client"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/st-matskevich/go-matchmaker/common"
	"github.com/st-matskevich/go-matchmaker/maker/processor"
)

func main() {
	log.Println("Starting Maker service")

	err := godotenv.Load(".env")
	if err != nil {
		log.Println("No .env file found")
	}

	clientDocker, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Docker connection error: %v", err)
	}
	defer clientDocker.Close()
	log.Println("Connected to Docker")

	redisServerURL := os.Getenv("REDIS_SERVER_URL")
	clientRedis := redis.NewClient(&redis.Options{
		Addr: redisServerURL,
		DB:   common.REDIS_DB_ID,
	})
	defer clientRedis.Close()

	ctx := context.Background()
	_, err = clientRedis.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Redis connection error: %v", err)
	}

	log.Println("Connected to Redis")

	maxJobs, err := strconv.Atoi(os.Getenv("MAX_CONCURRENT_JOBS"))
	if err != nil {
		log.Fatalf("Failed to MAX_CONCURRENT_JOBS: %v", err)
	}

	processor := processor.Processor{}
	err = processor.Init(clientRedis, clientDocker)
	if err != nil {
		log.Fatalf("Failed to initialize Processor: %v", err)
	}

	log.Printf("Starting processing messages in %v jobs", maxJobs)

	waitChan := make(chan struct{}, maxJobs)
	for {
		waitChan <- struct{}{}
		go func() {
			val, err := clientRedis.BRPop(ctx, 0, common.REDIS_QUEUE_LIST_KEY).Result()
			if err != nil {
				log.Printf("Redis brpop error: %v", err)
			}

			message := val[1]
			err = processor.ProcessMessage(message)
			if err != nil {
				log.Printf("Failed to process message (%v): %v", message, err)
			}

			<-waitChan
		}()
	}
}
