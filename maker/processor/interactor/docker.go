package interactor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

type DockerInteractor struct {
	DockerClient *client.Client

	ImageRegistryUsername string
	ImageRegisrtyPassword string

	DockerNetwork    string
	ImageName        string
	ImageExposedPort nat.Port
}

func (interactor *DockerInteractor) ListContainers() ([]string, error) {
	result := []string{}
	ctx := context.Background()

	args := filters.NewArgs(filters.KeyValuePair{Key: "ancestor", Value: interactor.ImageName}, filters.KeyValuePair{Key: "status", Value: "running"})
	containers, err := interactor.DockerClient.ContainerList(ctx, types.ContainerListOptions{Filters: args})
	if err != nil {
		return result, err
	}

	for _, container := range containers {
		result = append(result, container.ID)
	}

	return result, nil
}

func (interactor *DockerInteractor) InspectContainer(id string) (ContainerInfo, error) {
	result := ContainerInfo{}
	ctx := context.Background()

	containerInfo, err := interactor.DockerClient.ContainerInspect(ctx, id)
	if err != nil {
		return result, err
	}

	binding := containerInfo.NetworkSettings.Ports[interactor.ImageExposedPort]
	if len(binding) == 0 {
		return result, errors.New("no binding found for specified IMAGE_EXPOSE_PORT")
	}

	result.Address = containerInfo.Config.Hostname
	result.ExposedPort = binding[0].HostPort

	return result, nil
}

func (interactor *DockerInteractor) CreateContainer() (string, error) {
	ctx := context.Background()
	pullOptions := types.ImagePullOptions{}
	if interactor.ImageRegistryUsername != "" {
		authConfig := types.AuthConfig{
			Username: interactor.ImageRegistryUsername,
			Password: interactor.ImageRegisrtyPassword,
		}

		encodedJSON, err := json.Marshal(authConfig)
		if err != nil {
			return "", err
		}

		pullOptions.RegistryAuth = base64.URLEncoding.EncodeToString(encodedJSON)
	}

	log.Printf("Pulling image %v", interactor.ImageName)
	out, err := interactor.DockerClient.ImagePull(ctx, interactor.ImageName, pullOptions)
	if err != nil {
		return "", err
	}

	_, err = io.Copy(os.Stdout, out)
	if err != nil {
		return "", err
	}

	err = out.Close()
	if err != nil {
		return "", err
	}

	log.Println("Image pulled")

	//using limited port range for container port mapping would be much more correct, but:
	//1) scanning even 1k ports takes much more time then -P(ublish)
	//2) docker can fail binding if any(!) of ports in range is already binded
	//looks like OS is much better in ports management, so to limit available ports using
	///proc/sys/net/ipv4/ip_local_port_range
	hostConfig := container.HostConfig{}
	portBindings := []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "0"}}
	hostConfig.PortBindings = make(nat.PortMap)
	hostConfig.PortBindings[interactor.ImageExposedPort] = portBindings

	hostConfig.NetworkMode = container.NetworkMode(interactor.DockerNetwork)

	log.Println("Creating continer")
	resp, err := interactor.DockerClient.ContainerCreate(ctx, &container.Config{Image: interactor.ImageName}, &hostConfig, nil, nil, "")
	if err != nil {
		return "", err
	}
	log.Printf("Created container %v", resp.ID)

	err = interactor.DockerClient.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}
