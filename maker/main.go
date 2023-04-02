package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/st-matskevich/go-matchmaker/common"
	"github.com/st-matskevich/go-matchmaker/maker/processor"
	"github.com/st-matskevich/go-matchmaker/maker/processor/interactor"
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

	processor, err := initProcessor(clientRedis, clientDocker)
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

func initProcessor(redis *redis.Client, docker *client.Client) (*processor.Processor, error) {
	numberString := os.Getenv("RESERVATION_TIMEOUT")
	reservationTimeout, err := strconv.Atoi(numberString)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Timeout: time.Duration(reservationTimeout) * time.Millisecond}

	numberString = os.Getenv("RESERVATION_COOLDOWN")
	reservationCooldown, err := strconv.Atoi(numberString)
	if err != nil {
		return nil, err
	}

	numberString = os.Getenv("RESERVATION_RETRY_TIMES")
	reservationRetries, err := strconv.Atoi(numberString)
	if err != nil {
		return nil, err
	}

	imageName := os.Getenv("IMAGE_TO_PULL")
	dockerNetwork := os.Getenv("DOCKER_NETWORK")

	imageControlPort := os.Getenv("IMAGE_CONTROL_PORT")
	imageExposedPortString := os.Getenv("IMAGE_EXPOSE_PORT")
	exposedPort, err := nat.NewPort(nat.SplitProtoPort(imageExposedPortString))
	if err != nil {
		return nil, err
	}
	imageExposedPort := exposedPort

	imageRegistryUsername := os.Getenv("IMAGE_REGISTRY_USERNAME")
	imageRegisrtyPassword := os.Getenv("IMAGE_REGISTRY_PASSWORD")

	cooldownString := os.Getenv("LOOKUP_COOLDOWN")
	lookupCooldown, err := strconv.Atoi(cooldownString)
	if err != nil {
		return nil, err
	}

	numberString = os.Getenv("CONVERGE_VERIFY_COOLDOWN")
	convergeVerifyCooldown, err := strconv.Atoi(numberString)
	if err != nil {
		return nil, err
	}

	numberString = os.Getenv("CONVERGE_VERIFY_RETRY_TIMES")
	convergeVerifyRetries, err := strconv.Atoi(numberString)
	if err != nil {
		return nil, err
	}

	dockerInteractor := interactor.SwarmInteractor{
		DockerClient:           docker,
		ImageRegistryUsername:  imageRegistryUsername,
		ImageRegisrtyPassword:  imageRegisrtyPassword,
		DockerNetwork:          dockerNetwork,
		ImageName:              imageName,
		ImageExposedPort:       imageExposedPort,
		ConvergeVerifyCooldown: convergeVerifyCooldown,
		ConvergeVerifyRetries:  convergeVerifyRetries,
	}

	return &processor.Processor{
		RedisClient:         redis,
		DockerClient:        &dockerInteractor,
		HttpClient:          httpClient,
		ImageControlPort:    imageControlPort,
		LookupCooldown:      lookupCooldown,
		ReservationCooldown: reservationCooldown,
		ReservationRetries:  reservationRetries,
	}, nil
}
