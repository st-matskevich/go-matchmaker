package main

import (
	"log"
	"os"
	"strconv"

	"github.com/go-redis/redis"
	"github.com/st-matskevich/go-matchmaker/common"
)

func main() {
	log.Println("Starting Maker service")

	redisServerURL := os.Getenv("REDIS_SERVER_URL")
	clientRedis := redis.NewClient(&redis.Options{
		Addr:     redisServerURL,
		Password: "",
		DB:       0,
	})
	defer clientRedis.Close()

	_, err := clientRedis.Ping().Result()
	if err != nil {
		log.Fatalf("Redis connection error: %v", err)
	}

	log.Println("Successfully connected to Redis")

	maxJobs, err := strconv.Atoi(os.Getenv("MAX_CONCURRENT_JOBS"))
	if err != nil {
		log.Fatalf("Failed to MAX_CONCURRENT_JOBS: %v", err)
	}

	processor := Processor{redisClient: clientRedis}

	log.Printf("Starting processing messages in %v jobs", maxJobs)

	waitChan := make(chan struct{}, maxJobs)
	count := 0
	for {
		waitChan <- struct{}{}
		count++
		go func(count int) {
			val, err := clientRedis.BRPop(0, common.REDIS_QUEUE_LIST_KEY).Result()
			if err != nil {
				log.Println("RabbitMQ messages channel closed")
			}

			message := val[1]
			err = processor.ProcessMessage(message)
			if err != nil {
				log.Printf("Failed to process message (%v): %v", message, err)
			}

			<-waitChan
		}(count)
	}
}
