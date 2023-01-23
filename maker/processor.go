package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"os"
	"strconv"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
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

	err = processor.StartContainer()
	if err != nil {
		return err
	}

	log.Printf("Finished request: %v", request.ID)

	request.Status = common.DONE
	err = processor.WriteRequest(&request)
	if err != nil {
		return err
	}

	log.Printf("Set request %v status to DONE", request.ID)

	return nil
}

func (processor *Processor) StartContainer() error {
	ctx := context.Background()

	pullOptions := types.ImagePullOptions{}
	if os.Getenv("IMAGE_REGISTRY_USERNAME") != "" {
		authConfig := types.AuthConfig{
			Username: os.Getenv("IMAGE_REGISTRY_USERNAME"),
			Password: os.Getenv("IMAGE_REGISTRY_PASSWORD"),
		}

		encodedJSON, err := json.Marshal(authConfig)
		if err != nil {
			return err
		}

		pullOptions.RegistryAuth = base64.URLEncoding.EncodeToString(encodedJSON)
	}

	imageName := os.Getenv("IMAGE_TO_PULL")
	log.Printf("Pulling image %v", imageName)
	out, err := processor.dockerClient.ImagePull(ctx, imageName, pullOptions)
	if err != nil {
		return err
	}
	defer out.Close()
	log.Println("Image pulled")

	hostConfig := container.HostConfig{}
	//hostConfig.PortBindings is not allowing to pick random port
	hostConfig.PublishAllPorts = true

	log.Println("Creating continer")
	resp, err := processor.dockerClient.ContainerCreate(ctx, &container.Config{
		Image: imageName,
	}, &hostConfig, nil, nil, "")
	if err != nil {
		return err
	}
	log.Printf("Created container %v", resp.ID)

	log.Println("Starting container")
	if err := processor.dockerClient.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return err
	}
	log.Println("Container started")

	return nil
}
