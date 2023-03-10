package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
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

	app := fiber.New()

	app.Use(
		auth.New(&auth.DummyAuthorizer{}),
	)

	controller, err := initController(clientRedis)
	if err != nil {
		log.Fatalf("Failed to initialize Controller: %v", err)
	}

	app.Post("/request", controller.HandleCreateRequest)

	log.Fatal(app.Listen(":3000"))
}

func initController(redis *redis.Client) (*controller.Controller, error) {
	timeoutString := os.Getenv("RESERVATION_TIMEOUT")
	reservationTimeout, err := strconv.Atoi(timeoutString)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Timeout: time.Duration(reservationTimeout) * time.Millisecond}

	imageControlPort := os.Getenv("IMAGE_CONTROL_PORT")

	return &controller.Controller{
		RedisClient:      redis,
		HttpClient:       httpClient,
		ImageControlPort: imageControlPort,
	}, nil
}
