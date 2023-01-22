package main

import (
	"log"
	"os"

	"github.com/go-redis/redis"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/sony/sonyflake"
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
	connectRMQ, err := amqp.Dial(amqpServerURL)
	if err != nil {
		log.Fatalf("RabbitMQ connection error: %v", err)
	}
	defer connectRMQ.Close()

	channelRMQ, err := connectRMQ.Channel()
	if err != nil {
		log.Fatalf("RabbitMQ channel open error: %v", err)
	}
	defer channelRMQ.Close()

	_, err = channelRMQ.QueueDeclare(
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

	app := fiber.New()

	app.Use(
		logger.New(),
	)

	controller := Controller{idGenerator: sf, rmqChannel: channelRMQ, redisClient: clientRedis}
	app.Post("/request", controller.HandleCreateRequest)
	app.Get("/request/:id", controller.HandleGetRequest)

	log.Fatal(app.Listen(":3000"))
}
