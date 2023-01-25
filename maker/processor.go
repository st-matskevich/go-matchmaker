package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strconv"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/go-redis/redis"
	"github.com/st-matskevich/go-matchmaker/common"
)

type Processor struct {
	redisClient  *redis.Client
	dockerClient *client.Client
}

func (processor *Processor) WriteRequest(req *common.RequestBody) error {
	stringID := strconv.FormatUint(req.ID, 10)
	bytes, err := json.Marshal(req)
	if err != nil {
		return err
	}

	processor.redisClient.Set(stringID, string(bytes), 0).Err()
	if err != nil {
		return err
	}

	return nil
}

func (processor *Processor) ProcessMessage(message string) error {
	var request common.RequestBody
	err := json.Unmarshal([]byte(message), &request)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			request.Status = common.FAILED
			processor.WriteRequest(&request)
		}
	}()

	log.Printf("Got request: %v", request)

	request.Status = common.IN_PROGRESS
	err = processor.WriteRequest(&request)
	if err != nil {
		return err
	}

	log.Printf("Set request %v status to IN_PROGRESS", request.ID)

	containerPort, err := processor.StartNewContainer()
	if err != nil {
		return err
	}

	request.Server = "localhost:" + containerPort

	log.Printf("Finished request: %v", request.ID)

	request.Status = common.DONE
	err = processor.WriteRequest(&request)
	if err != nil {
		return err
	}

	log.Printf("Set request %v status to DONE", request.ID)

	return nil
}

func (processor *Processor) StartNewContainer() (string, error) {
	ctx := context.Background()

	log.Printf("Looking for exited containers")

	imageName := os.Getenv("IMAGE_TO_PULL")
	args := filters.NewArgs(filters.KeyValuePair{Key: "ancestor", Value: imageName}, filters.KeyValuePair{Key: "status", Value: "exited"})
	containers, err := processor.dockerClient.ContainerList(ctx, types.ContainerListOptions{Filters: args})
	if err != nil {
		return "", err
	}

	for _, container := range containers {
		log.Printf("Found exited container %v", container.ID)
		return processor.StartContainer(ctx, container.ID)
	}

	log.Printf("No exited containers available, starting new one")

	return processor.CreateNewContainer()
}

func (processor *Processor) CreateNewContainer() (string, error) {
	ctx := context.Background()

	pullOptions := types.ImagePullOptions{}
	if os.Getenv("IMAGE_REGISTRY_USERNAME") != "" {
		authConfig := types.AuthConfig{
			Username: os.Getenv("IMAGE_REGISTRY_USERNAME"),
			Password: os.Getenv("IMAGE_REGISTRY_PASSWORD"),
		}

		encodedJSON, err := json.Marshal(authConfig)
		if err != nil {
			return "", err
		}

		pullOptions.RegistryAuth = base64.URLEncoding.EncodeToString(encodedJSON)
	}

	imageName := os.Getenv("IMAGE_TO_PULL")
	log.Printf("Pulling image %v", imageName)
	out, err := processor.dockerClient.ImagePull(ctx, imageName, pullOptions)
	if err != nil {
		return "", err
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
	hostConfig.NetworkMode = container.NetworkMode(os.Getenv("DOCKER_NETWORK"))

	log.Println("Creating continer")
	resp, err := processor.dockerClient.ContainerCreate(ctx, &container.Config{
		Image: imageName,
	}, &hostConfig, nil, nil, "")
	if err != nil {
		return "", err
	}
	log.Printf("Created container %v", resp.ID)

	return processor.StartContainer(ctx, resp.ID)
}

func (processor *Processor) StartContainer(ctx context.Context, ID string) (string, error) {
	log.Println("Starting container")
	err := processor.dockerClient.ContainerStart(ctx, ID, types.ContainerStartOptions{})
	if err != nil {
		return "", err
	}

	containerInfo, err := processor.dockerClient.ContainerInspect(ctx, ID)
	if err != nil {
		return "", err
	}

	imagePort := os.Getenv("IMAGE_PORT")
	port, err := nat.NewPort(nat.SplitProtoPort(imagePort))
	if err != nil {
		return "", err
	}

	//containerInfo.Config.Hostname and port.Port() can be used to access started container
	binding := containerInfo.NetworkSettings.Ports[port]
	if len(binding) == 0 {
		return "", errors.New("no binding found for specified IMAGE_PORT")
	}

	hostPort := binding[0].HostPort
	log.Printf("Container started on port %v", hostPort)

	return hostPort, nil
}
