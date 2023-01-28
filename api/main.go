package main

import (
	"log"
	"os"

	"github.com/go-redis/redis"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/joho/godotenv"
	"github.com/sony/sonyflake"
	"github.com/st-matskevich/go-matchmaker/api/auth"
	"github.com/st-matskevich/go-matchmaker/api/controller"
	"github.com/st-matskevich/go-matchmaker/common"
)

func main() {
	log.Println("Starting API service")

	err := godotenv.Load(".env")
	if err != nil {
		log.Println("No .env file found")
	}

	var st sonyflake.Settings
	sf := sonyflake.NewSonyflake(st)
	if sf == nil {
		log.Fatalf("Sonyflake initialization failed")
	}

	redisServerURL := os.Getenv("REDIS_SERVER_URL")
	clientRedis := redis.NewClient(&redis.Options{
		Addr: redisServerURL,
		DB:   common.REDIS_DB_ID,
	})
	defer clientRedis.Close()

	_, err = clientRedis.Ping().Result()
	if err != nil {
		log.Fatalf("Redis connection error: %v", err)
	}

	log.Println("Connected to Redis")

	app := fiber.New()

	app.Use(
		logger.New(),
		auth.New(&auth.DummyAuthorizer{}),
	)

	controller := controller.Controller{}
	controller.Init(sf, clientRedis)
	app.Post("/request", controller.HandleCreateRequest)

	log.Fatal(app.Listen(":3000"))
}
