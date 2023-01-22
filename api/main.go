package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/sony/sonyflake"
	"github.com/st-matskevich/go-matchmaker/common"
	"github.com/streadway/amqp"
)

func main() {
	log.Println("Starting API service")

	var st sonyflake.Settings
	sf := sonyflake.NewSonyflake(st)
	if sf == nil {
		log.Fatalf("Sonyflake initialization failed")
	}

	amqpServerURL := os.Getenv("AMQP_SERVER_URL")
	connectRabbitMQ, err := amqp.Dial(amqpServerURL)
	if err != nil {
		log.Fatalf("RabbitMQ connection error: %v", err)
	}
	defer connectRabbitMQ.Close()

	channelRabbitMQ, err := connectRabbitMQ.Channel()
	if err != nil {
		log.Fatalf("RabbitMQ channel open error: %v", err)
	}
	defer channelRabbitMQ.Close()

	_, err = channelRabbitMQ.QueueDeclare(
		"MakerQueue", // queue name
		false,        // durable
		false,        // auto delete
		false,        // exclusive
		false,        // no wait
		nil,          // arguments
	)
	if err != nil {
		log.Fatalf("RabbitMQ queue declare error: %v", err)
	}

	log.Println("Successfully connected to RabbitMQ")

	app := fiber.New()

	app.Use(
		logger.New(),
	)

	app.Post("/request", func(c *fiber.Ctx) error {
		id, err := sf.NextID()
		if err != nil {
			log.Printf("Sonyflake id generation error: %v", err)
			return c.SendStatus(fiber.StatusInternalServerError)
		}

		body := common.RequestBody{ID: id}
		bytes, err := json.Marshal(body)
		if err != nil {
			log.Printf("JSON encoder error: %v", err)
			return c.SendStatus(fiber.StatusInternalServerError)
		}

		message := amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte(bytes),
		}

		if err := channelRabbitMQ.Publish(
			"",           // exchange
			"MakerQueue", // queue name
			false,        // mandatory
			false,        // immediate
			message,      // message to publish
		); err != nil {
			log.Printf("RabbitMQ message post error: %v", err)
			return c.SendStatus(fiber.StatusInternalServerError)
		}

		return c.Status(fiber.StatusAccepted).SendString(string(bytes))
	})

	log.Fatal(app.Listen(":3000"))
}
