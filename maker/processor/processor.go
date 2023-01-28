package processor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/redis/go-redis/v9"
	"github.com/st-matskevich/go-matchmaker/common"
)

type Processor struct {
	redisClient  *redis.Client
	dockerClient *client.Client
	httpClient   *http.Client
	creatorMutex sync.Mutex

	imageName        string
	dockerNetwork    string
	imageControlPort string
	imageExposedPort nat.Port

	imageRegistryUsername string
	imageRegisrtyPassword string

	lookupCooldownMillisecond int
}

type ContainerInfo struct {
	Hostname    string
	ExposedPort string
}

func FillRequestWithContainerInfo(request *common.RequestBody, info *ContainerInfo) {
	request.Container = info.Hostname
	request.Server = "localhost:" + info.ExposedPort
}

func (processor *Processor) Init(redis *redis.Client, docker *client.Client) error {
	processor.redisClient = redis
	processor.dockerClient = docker

	timeoutString := os.Getenv("RESERVATION_TIMEOUT")
	reservationTimeout, err := strconv.Atoi(timeoutString)
	if err != nil {
		return err
	}
	processor.httpClient = &http.Client{Timeout: time.Duration(reservationTimeout) * time.Millisecond}

	processor.imageName = os.Getenv("IMAGE_TO_PULL")
	processor.dockerNetwork = os.Getenv("DOCKER_NETWORK")

	processor.imageControlPort = os.Getenv("IMAGE_CONTROL_PORT")
	imageExposedPortString := os.Getenv("IMAGE_EXPOSE_PORT")
	exposedPort, err := nat.NewPort(nat.SplitProtoPort(imageExposedPortString))
	if err != nil {
		return err
	}
	processor.imageExposedPort = exposedPort

	processor.imageRegistryUsername = os.Getenv("IMAGE_REGISTRY_USERNAME")
	processor.imageRegisrtyPassword = os.Getenv("IMAGE_REGISTRY_PASSWORD")

	cooldownString := os.Getenv("LOOKUP_COOLDOWN")
	processor.lookupCooldownMillisecond, err = strconv.Atoi(cooldownString)
	if err != nil {
		return err
	}

	return nil
}

func (processor *Processor) WriteRequest(ctx context.Context, req *common.RequestBody) error {
	bytes, err := json.Marshal(req)
	if err != nil {
		return err
	}

	processor.redisClient.Set(ctx, req.ID, string(bytes), 0).Err()
	if err != nil {
		return err
	}

	return nil
}

func (processor *Processor) ProcessMessage(message string) (rerr error) {
	var request common.RequestBody
	ctx := context.Background()
	err := json.Unmarshal([]byte(message), &request)
	if err != nil {
		return err
	}

	defer func() {
		perr := recover()
		if perr != nil || rerr != nil {
			request.Status = common.FAILED
			processor.WriteRequest(ctx, &request)
		}
	}()

	log.Printf("Got request: %v", request)

	request.Status = common.IN_PROGRESS
	err = processor.WriteRequest(ctx, &request)
	if err != nil {
		return err
	}

	log.Printf("Set request %v status to IN_PROGRESS", request.ID)

	for {
		containerInfo, err := processor.FindRunningContainer(ctx, request.ID)
		if err != nil {
			return err
		}

		if containerInfo.ExposedPort != "" {
			FillRequestWithContainerInfo(&request, &containerInfo)
			break
		}

		if processor.creatorMutex.TryLock() {
			defer processor.creatorMutex.Unlock()
			containerInfo, err = processor.StartNewContainer(ctx, request.ID)
			if err != nil {
				return err
			}

			if containerInfo.ExposedPort == "" {
				return errors.New("StartNewContainer didn't return port")
			}

			FillRequestWithContainerInfo(&request, &containerInfo)
			break
		}

		time.Sleep(time.Duration(processor.lookupCooldownMillisecond) * time.Millisecond)
	}

	log.Printf("Finished request: %v", request.ID)

	request.Status = common.DONE
	err = processor.WriteRequest(ctx, &request)
	if err != nil {
		return err
	}

	log.Printf("Set request %v status to DONE", request.ID)

	return nil
}

func (processor *Processor) FindRunningContainer(ctx context.Context, requestID string) (ContainerInfo, error) {
	result := ContainerInfo{}

	log.Printf("Looking for available containers")

	args := filters.NewArgs(filters.KeyValuePair{Key: "ancestor", Value: processor.imageName}, filters.KeyValuePair{Key: "status", Value: "running"})
	containers, err := processor.dockerClient.ContainerList(ctx, types.ContainerListOptions{Filters: args})
	if err != nil {
		return result, err
	}

	for _, container := range containers {
		containerInfo, err := processor.dockerClient.ContainerInspect(ctx, container.ID)
		if err != nil {
			log.Printf("Failed ContainerInspect on container %v: %v", container.ID, err)
			continue
		}

		reserved, err := processor.ReserveContainer(containerInfo.Config.Hostname, requestID)
		if err != nil {
			log.Printf("Failed reserve request on container %v: %v", container.ID, err)
			continue
		}

		if reserved {
			log.Printf("Found available container %v", container.ID)
			port, err := processor.GetContainerExposedPort(&containerInfo)
			if err != nil {
				log.Printf("Failed exposed port parse on container %v: %v", container.ID, err)
				continue
			}

			result.Hostname = containerInfo.Config.Hostname
			result.ExposedPort = port
			return result, nil
		}
	}

	log.Printf("No available containers found")

	return result, nil
}

func (processor *Processor) StartNewContainer(ctx context.Context, requestID string) (ContainerInfo, error) {
	result := ContainerInfo{}

	log.Printf("Looking for exited containers")

	args := filters.NewArgs(filters.KeyValuePair{Key: "ancestor", Value: processor.imageName}, filters.KeyValuePair{Key: "status", Value: "exited"})
	containers, err := processor.dockerClient.ContainerList(ctx, types.ContainerListOptions{Filters: args})
	if err != nil {
		return result, err
	}

	if len(containers) > 0 {
		container := containers[0]
		log.Printf("Found exited container %v", container.ID)
		return processor.StartContainer(ctx, requestID, container.ID)
	}

	log.Printf("No exited containers available, starting new one")

	return processor.CreateNewContainer(ctx, requestID)
}

func (processor *Processor) CreateNewContainer(ctx context.Context, requestID string) (ContainerInfo, error) {
	result := ContainerInfo{}
	pullOptions := types.ImagePullOptions{}
	if processor.imageRegistryUsername != "" {
		authConfig := types.AuthConfig{
			Username: processor.imageRegistryUsername,
			Password: processor.imageRegisrtyPassword,
		}

		encodedJSON, err := json.Marshal(authConfig)
		if err != nil {
			return result, err
		}

		pullOptions.RegistryAuth = base64.URLEncoding.EncodeToString(encodedJSON)
	}

	log.Printf("Pulling image %v", processor.imageName)
	out, err := processor.dockerClient.ImagePull(ctx, processor.imageName, pullOptions)
	if err != nil {
		return result, err
	}
	defer out.Close()
	log.Println("Image pulled")

	//using limited port range for container port mapping would be much more correct, but:
	//1) scanning even 1k ports takes much more time then -P(ublish)
	//2) docker can fail binding if any(!) of ports in range is already binded
	//looks like OS is much better in ports management, so to limit available ports using
	///proc/sys/net/ipv4/ip_local_port_range
	hostConfig := container.HostConfig{}
	hostConfig.PublishAllPorts = true
	hostConfig.NetworkMode = container.NetworkMode(processor.dockerNetwork)

	log.Println("Creating continer")
	resp, err := processor.dockerClient.ContainerCreate(ctx, &container.Config{Image: processor.imageName}, &hostConfig, nil, nil, "")
	if err != nil {
		return result, err
	}
	log.Printf("Created container %v", resp.ID)

	return processor.StartContainer(ctx, requestID, resp.ID)
}

func (processor *Processor) StartContainer(ctx context.Context, requestID string, containerID string) (ContainerInfo, error) {
	result := ContainerInfo{}

	log.Println("Starting container")
	err := processor.dockerClient.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
	if err != nil {
		return result, err
	}

	containerInfo, err := processor.dockerClient.ContainerInspect(ctx, containerID)
	if err != nil {
		return result, err
	}

	hostPort, err := processor.GetContainerExposedPort(&containerInfo)
	if err != nil {
		return result, err
	}

	log.Printf("Container started on port %v", hostPort)

	reserved, err := processor.ReserveContainer(containerInfo.Config.Hostname, requestID)
	if err != nil {
		return result, err
	}

	if !reserved {
		return result, errors.New("container failed to reserve a slot")
	}

	result.Hostname = containerInfo.Config.Hostname
	result.ExposedPort = hostPort
	return result, nil
}

func (processor *Processor) GetContainerExposedPort(containerInfo *types.ContainerJSON) (string, error) {
	binding := containerInfo.NetworkSettings.Ports[processor.imageExposedPort]
	if len(binding) == 0 {
		return "", errors.New("no binding found for specified IMAGE_EXPOSE_PORT")
	}

	return binding[0].HostPort, nil
}

func (processor *Processor) ReserveContainer(hostname string, requestID string) (bool, error) {
	containerURL := "http://" + hostname + ":" + processor.imageControlPort
	containerURL += "/reservation/" + requestID
	resp, err := processor.httpClient.Post(containerURL, "*", nil)
	if err != nil {
		return false, err
	}

	return resp.StatusCode == 200, nil
}
