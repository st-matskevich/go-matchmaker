package main

import (
	"log"
	"os"

	"github.com/go-redis/redis"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/sony/sonyflake"
)

func main() {
	log.Println("Starting API service")

	var st sonyflake.Settings
	sf := sonyflake.NewSonyflake(st)
	if sf == nil {
		log.Fatalf("Sonyflake initialization failed")
	}

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

	app := fiber.New()

	app.Use(
		logger.New(),
	)

	controller := Controller{idGenerator: sf, redisClient: clientRedis}
	app.Post("/request", controller.HandleCreateRequest)
	app.Get("/request/:id", controller.HandleGetRequest)

	log.Fatal(app.Listen(":3000"))
}
