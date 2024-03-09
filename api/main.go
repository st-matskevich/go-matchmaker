package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
	"github.com/st-matskevich/go-matchmaker/api/auth"
	"github.com/st-matskevich/go-matchmaker/api/controller"
	"github.com/st-matskevich/go-matchmaker/common/data"
)

func main() {
	log.Println("Starting API service")

	err := godotenv.Load(".env")
	if err != nil {
		log.Println("No .env file found")
	}

	redisServerURL := os.Getenv("REDIS_SERVER_URL")
	clientRedis, err := data.CreateRedisDataProvider(redisServerURL)
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

func initController(dataProvider data.DataProvider) (*controller.Controller, error) {
	timeoutString := os.Getenv("RESERVATION_TIMEOUT")
	reservationTimeout, err := strconv.Atoi(timeoutString)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Timeout: time.Duration(reservationTimeout) * time.Millisecond}

	imageControlPort := os.Getenv("IMAGE_CONTROL_PORT")

	return &controller.Controller{
		DataProvider:     dataProvider,
		HttpClient:       httpClient,
		ImageControlPort: imageControlPort,
	}, nil
}
