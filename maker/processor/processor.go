package processor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/go-connections/nat"
	"github.com/st-matskevich/go-matchmaker/common"
	"github.com/st-matskevich/go-matchmaker/common/interfaces"
)

type Processor struct {
	RedisClient  interfaces.RedisClient
	DockerClient DockerClient
	HttpClient   interfaces.HTTPClient
	creatorMutex sync.Mutex

	Hostname         string
	ImageName        string
	DockerNetwork    string
	ImageControlPort string
	ImageExposedPort nat.Port

	ImageRegistryUsername string
	ImageRegisrtyPassword string

	LookupCooldownMillisecond int
}

type ContainerInfo struct {
	Hostname    string
	ExposedPort string
}

func (processor *Processor) fillRequestWithContainerInfo(request *common.RequestBody, info *ContainerInfo) {
	request.Container = info.Hostname
	request.Server = processor.Hostname + info.ExposedPort
}

func (processor *Processor) writeRequest(ctx context.Context, req *common.RequestBody) error {
	bytes, err := json.Marshal(req)
	if err != nil {
		return err
	}

	err = processor.RedisClient.Set(ctx, req.ID, string(bytes), 0).Err()
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
			if rerr == nil {
				switch x := perr.(type) {
				case string:
					rerr = errors.New(x)
				case error:
					rerr = x
				default:
					rerr = errors.New("unknown panic")
				}
			}
			request.Status = common.FAILED
			processor.writeRequest(ctx, &request)
		}
	}()

	log.Printf("Got request: %v", request)

	request.Status = common.IN_PROGRESS
	err = processor.writeRequest(ctx, &request)
	if err != nil {
		return err
	}

	log.Printf("Set request %v status to IN_PROGRESS", request.ID)

	for {
		containerInfo, err := processor.findRunningContainer(ctx, request.ID)
		if err != nil {
			return err
		}

		if containerInfo.ExposedPort != "" {
			processor.fillRequestWithContainerInfo(&request, &containerInfo)
			break
		}

		if processor.creatorMutex.TryLock() {
			defer processor.creatorMutex.Unlock()
			containerInfo, err = processor.startNewContainer(ctx, request.ID)
			if err != nil {
				return err
			}

			if containerInfo.ExposedPort == "" {
				return errors.New("StartNewContainer didn't return port")
			}

			processor.fillRequestWithContainerInfo(&request, &containerInfo)
			break
		}

		time.Sleep(time.Duration(processor.LookupCooldownMillisecond) * time.Millisecond)
	}

	log.Printf("Finished request: %v", request.ID)

	request.Status = common.DONE
	err = processor.writeRequest(ctx, &request)
	if err != nil {
		return err
	}

	log.Printf("Set request %v status to DONE", request.ID)

	return nil
}

func (processor *Processor) findRunningContainer(ctx context.Context, requestID string) (ContainerInfo, error) {
	result := ContainerInfo{}

	log.Printf("Looking for available containers")

	args := filters.NewArgs(filters.KeyValuePair{Key: "ancestor", Value: processor.ImageName}, filters.KeyValuePair{Key: "status", Value: "running"})
	containers, err := processor.DockerClient.ContainerList(ctx, types.ContainerListOptions{Filters: args})
	if err != nil {
		return result, err
	}

	for _, container := range containers {
		containerInfo, err := processor.DockerClient.ContainerInspect(ctx, container.ID)
		if err != nil {
			log.Printf("Failed ContainerInspect on container %v: %v", container.ID, err)
			continue
		}

		reserved, err := processor.reserveContainer(containerInfo.Config.Hostname, requestID)
		if err != nil {
			log.Printf("Failed reserve request on container %v: %v", container.ID, err)
			continue
		}

		if reserved {
			log.Printf("Found available container %v", container.ID)
			port, err := processor.getContainerExposedPort(&containerInfo)
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

func (processor *Processor) startNewContainer(ctx context.Context, requestID string) (ContainerInfo, error) {
	result := ContainerInfo{}

	log.Printf("Looking for exited containers")

	args := filters.NewArgs(filters.KeyValuePair{Key: "ancestor", Value: processor.ImageName}, filters.KeyValuePair{Key: "status", Value: "exited"})
	containers, err := processor.DockerClient.ContainerList(ctx, types.ContainerListOptions{Filters: args})
	if err != nil {
		return result, err
	}

	if len(containers) > 0 {
		container := containers[0]
		log.Printf("Found exited container %v", container.ID)
		return processor.startContainer(ctx, requestID, container.ID)
	}

	log.Printf("No exited containers available, starting new one")

	return processor.createNewContainer(ctx, requestID)
}

func (processor *Processor) createNewContainer(ctx context.Context, requestID string) (ContainerInfo, error) {
	result := ContainerInfo{}
	pullOptions := types.ImagePullOptions{}
	if processor.ImageRegistryUsername != "" {
		authConfig := types.AuthConfig{
			Username: processor.ImageRegistryUsername,
			Password: processor.ImageRegisrtyPassword,
		}

		encodedJSON, err := json.Marshal(authConfig)
		if err != nil {
			return result, err
		}

		pullOptions.RegistryAuth = base64.URLEncoding.EncodeToString(encodedJSON)
	}

	log.Printf("Pulling image %v", processor.ImageName)
	out, err := processor.DockerClient.ImagePull(ctx, processor.ImageName, pullOptions)
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
	hostConfig.NetworkMode = container.NetworkMode(processor.DockerNetwork)

	log.Println("Creating continer")
	resp, err := processor.DockerClient.ContainerCreate(ctx, &container.Config{Image: processor.ImageName}, &hostConfig, nil, nil, "")
	if err != nil {
		return result, err
	}
	log.Printf("Created container %v", resp.ID)

	return processor.startContainer(ctx, requestID, resp.ID)
}

func (processor *Processor) startContainer(ctx context.Context, requestID string, containerID string) (ContainerInfo, error) {
	result := ContainerInfo{}

	log.Println("Starting container")
	err := processor.DockerClient.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
	if err != nil {
		return result, err
	}

	containerInfo, err := processor.DockerClient.ContainerInspect(ctx, containerID)
	if err != nil {
		return result, err
	}

	hostPort, err := processor.getContainerExposedPort(&containerInfo)
	if err != nil {
		return result, err
	}

	log.Printf("Container started on port %v", hostPort)

	reserved, err := processor.reserveContainer(containerInfo.Config.Hostname, requestID)
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

func (processor *Processor) getContainerExposedPort(containerInfo *types.ContainerJSON) (string, error) {
	binding := containerInfo.NetworkSettings.Ports[processor.ImageExposedPort]
	if len(binding) == 0 {
		return "", errors.New("no binding found for specified IMAGE_EXPOSE_PORT")
	}

	return binding[0].HostPort, nil
}

func (processor *Processor) reserveContainer(hostname string, requestID string) (bool, error) {
	containerURL := "http://" + hostname + ":" + processor.ImageControlPort
	containerURL += "/reservation/" + requestID

	req, err := http.NewRequest("POST", containerURL, nil)
	if err != nil {
		return false, err
	}

	resp, err := processor.HttpClient.Do(req)
	if err != nil {
		return false, err
	}

	return resp.StatusCode == 200, nil
}
