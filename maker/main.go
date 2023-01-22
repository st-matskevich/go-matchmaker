package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/st-matskevich/go-matchmaker/common"
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

	maxJobs, err := strconv.ParseInt(os.Getenv("MAX_CONCURRENT_JOBS"), 10, 32)
	if err != nil {
		log.Fatalf("Failed to MAX_CONCURRENT_JOBS: %v", err)
	}

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

			err := processMessage(message)
			if err != nil {
				log.Printf("Failed to process message (%v): %v", message, err)
			}

			<-waitChan
		}(count)
	}
}

func processMessage(message amqp.Delivery) error {
	var request common.RequestBody
	err := json.Unmarshal(message.Body, &request)
	if err != nil {
		return err
	}

	log.Printf("Got request: %v", request)

	//simulate some work
	time.Sleep(time.Duration((1000 + rand.Intn(2000)) * int(time.Millisecond)))

	log.Printf("Finished request: %v", request.ID)

	return nil
}
