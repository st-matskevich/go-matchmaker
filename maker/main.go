package main

import (
	"log"
	"os"
	"strconv"

	"github.com/go-redis/redis"
	"github.com/streadway/amqp"
)

func main() {
	log.Println("Starting Maker service")

	amqpServerURL := os.Getenv("AMQP_SERVER_URL")
	connectRabbitMQ, err := amqp.Dial(amqpServerURL)
	if err != nil {
		log.Fatalf("RabbitMQ connection error: %v", err)
	}
	defer connectRabbitMQ.Close()

	channelRabbitMQ, err := connectRabbitMQ.Channel()
	if err != nil {
		log.Fatalf("RabbitMQ channel open: %v", err)
	}
	defer channelRabbitMQ.Close()

	messages, err := channelRabbitMQ.Consume(
		"MakerQueue", // queue name
		"",           // consumer
		true,         // auto-ack
		false,        // exclusive
		false,        // no local
		false,        // no wait
		nil,          // arguments
	)
	if err != nil {
		log.Fatalf("RabbitMQ queue consume error: %v", err)
	}

	log.Println("Successfully connected to RabbitMQ")

	redisServerURL := os.Getenv("REDIS_SERVER_URL")
	clientRedis := redis.NewClient(&redis.Options{
		Addr:     redisServerURL,
		Password: "",
		DB:       0,
	})

	_, err = clientRedis.Ping().Result()
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
			message, ok := <-messages
			if !ok {
				log.Println("RabbitMQ messages channel closed")
			}

			err := processor.ProcessMessage(message)
			if err != nil {
				log.Printf("Failed to process message (%v): %v", message, err)
			}

			<-waitChan
		}(count)
	}
}
