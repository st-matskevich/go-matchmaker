package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

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

	redisServerURL := os.Getenv("REDIS_SERVER_URL")
	clientRedis, err := data.CreateRedisDataProvider(redisServerURL)
	if err != nil {
		log.Fatalf("Redis connection error: %v", err)
	}
	log.Println("Connected to Redis")

	image, err := getImageInfo()
	if err != nil {
		log.Fatalf("Failed to parse image info: %v", err)
	}
	log.Println("Parsed image info")

	containerInteractor, err := initInteractor(image)
	if err != nil {
		log.Fatalf("Failed to create container interactor: %v", err)
	}
	log.Println("Created container interactor")

	processor, err := initProcessor(image, clientRedis, containerInteractor)
	if err != nil {
		log.Fatalf("Failed to initialize Processor: %v", err)
	}

	log.Fatal(processor.Process())
}

func getImageInfo() (interactor.ImageInfo, error) {
	imageName := os.Getenv("IMAGE_TO_PULL")
	imageRegistryUsername := os.Getenv("IMAGE_REGISTRY_USERNAME")
	imageRegisrtyPassword := os.Getenv("IMAGE_REGISTRY_PASSWORD")

	portString := os.Getenv("IMAGE_EXPOSE_PORT")
	port, err := nat.NewPort(nat.SplitProtoPort(portString))
	if err != nil {
		return interactor.ImageInfo{}, err
	}
	imageExposedPort := port

	portString = os.Getenv("IMAGE_CONTROL_PORT")
	port, err = nat.NewPort(nat.SplitProtoPort(portString))
	if err != nil {
		return interactor.ImageInfo{}, err
	}
	imageControlPort := port

	return interactor.ImageInfo{
		ImageRegistryUsername: imageRegistryUsername,
		ImageRegisrtyPassword: imageRegisrtyPassword,
		ImageName:             imageName,
		ImageExposedPort:      imageExposedPort,
		ImageControlPort:      imageControlPort,
	}, nil
}

func initProcessor(image interactor.ImageInfo, dataProvider data.DataProvider, containerInteractor interactor.ContainerInteractor) (*processor.Processor, error) {
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

	cooldownString := os.Getenv("LOOKUP_COOLDOWN")
	lookupCooldown, err := strconv.Atoi(cooldownString)
	if err != nil {
		return nil, err
	}

	return &processor.Processor{
		DataProvider:        dataProvider,
		DockerClient:        containerInteractor,
		HttpClient:          httpClient,
		MaxJobs:             maxJobs,
		ImageControlPort:    image.ImageControlPort.Port(),
		LookupCooldown:      lookupCooldown,
		ReservationCooldown: reservationCooldown,
		ReservationRetries:  reservationRetries,
	}, nil
}

func initInteractor(image interactor.ImageInfo) (interactor.ContainerInteractor, error) {
	interactorType := os.Getenv("CONTAINER_BACKEND")
	switch interactorType {
	case interactor.DOCKER_INTERACTOR:
		log.Println("Starting on docker")

		dockerNetwork := os.Getenv("DOCKER_NETWORK")
		options := interactor.DockerContainerInteractorOptions{
			DockerNetwork: dockerNetwork,
		}

		interactor, err := interactor.CreateDockerContainerInteractor(image, options)
		if err != nil {
			return nil, err
		}

		return interactor, nil
	case interactor.SWARM_INTERACTOR:
		log.Println("Starting on swarm")

		dockerNetwork := os.Getenv("DOCKER_NETWORK")
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
		options := interactor.SwarmContainerInteractorOptions{
			DockerNetwork:          dockerNetwork,
			ConvergeVerifyCooldown: convergeVerifyCooldown,
			ConvergeVerifyRetries:  convergeVerifyRetries,
		}

		interactor, err := interactor.CreateSwarmContainerInteractor(image, options)
		if err != nil {
			return nil, err
		}

		return interactor, nil
	default:
		panic("unknown interactor type")
	}
}
