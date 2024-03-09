package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/joho/godotenv"
	"github.com/st-matskevich/go-matchmaker/common/data"
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
	clientRedis, err := data.CreateRedisDataProvider(redisServerURL)
	if err != nil {
		log.Fatalf("Redis connection error: %v", err)
	}

	log.Println("Connected to Redis")

	processor, err := initProcessor(clientRedis, clientDocker)
	if err != nil {
		log.Fatalf("Failed to initialize Processor: %v", err)
	}

	log.Fatal(processor.Process())
}

func initProcessor(dataProvider data.DataProvider, docker *client.Client) (*processor.Processor, error) {
	maxJobs, err := strconv.Atoi(os.Getenv("MAX_CONCURRENT_JOBS"))
	if err != nil {
		return nil, err
	}

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

	imageControlPort := os.Getenv("IMAGE_CONTROL_PORT")

	cooldownString := os.Getenv("LOOKUP_COOLDOWN")
	lookupCooldown, err := strconv.Atoi(cooldownString)
	if err != nil {
		return nil, err
	}

	dockerInteractor, err := initInteractor(docker)
	if err != nil {
		return nil, err
	}

	return &processor.Processor{
		DataProvider:        dataProvider,
		DockerClient:        dockerInteractor,
		HttpClient:          httpClient,
		MaxJobs:             maxJobs,
		ImageControlPort:    imageControlPort,
		LookupCooldown:      lookupCooldown,
		ReservationCooldown: reservationCooldown,
		ReservationRetries:  reservationRetries,
	}, nil
}

func initInteractor(docker *client.Client) (interactor.ContainerInteractor, error) {
	interactorType := os.Getenv("CONTAINER_BACKEND")

	imageName := os.Getenv("IMAGE_TO_PULL")
	imageRegistryUsername := os.Getenv("IMAGE_REGISTRY_USERNAME")
	imageRegisrtyPassword := os.Getenv("IMAGE_REGISTRY_PASSWORD")
	dockerNetwork := os.Getenv("DOCKER_NETWORK")

	imageExposedPortString := os.Getenv("IMAGE_EXPOSE_PORT")
	exposedPort, err := nat.NewPort(nat.SplitProtoPort(imageExposedPortString))
	if err != nil {
		return nil, err
	}
	imageExposedPort := exposedPort

	numberString := os.Getenv("CONVERGE_VERIFY_COOLDOWN")
	convergeVerifyCooldown, err := strconv.Atoi(numberString)
	if err != nil {
		return nil, err
	}

	numberString = os.Getenv("CONVERGE_VERIFY_RETRY_TIMES")
	convergeVerifyRetries, err := strconv.Atoi(numberString)
	if err != nil {
		return nil, err
	}

	switch interactorType {
	case interactor.DOCKER_INTERACTOR:
		log.Println("Starting on docker")
		return &interactor.DockerInteractor{
			DockerClient:          docker,
			ImageRegistryUsername: imageRegistryUsername,
			ImageRegisrtyPassword: imageRegisrtyPassword,
			DockerNetwork:         dockerNetwork,
			ImageName:             imageName,
			ImageExposedPort:      imageExposedPort,
		}, nil
	case interactor.SWARM_INTERACTOR:
		log.Println("Starting on swarm")
		return &interactor.SwarmInteractor{
			DockerClient:           docker,
			ImageRegistryUsername:  imageRegistryUsername,
			ImageRegisrtyPassword:  imageRegisrtyPassword,
			DockerNetwork:          dockerNetwork,
			ImageName:              imageName,
			ImageExposedPort:       imageExposedPort,
			ConvergeVerifyCooldown: convergeVerifyCooldown,
			ConvergeVerifyRetries:  convergeVerifyRetries,
		}, nil
	default:
		panic("unknown interactor type")
	}
}
