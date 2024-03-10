package interactor

import (
	"context"
	"errors"
	"io"
	"log"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const DOCKER_INTERACTOR = "docker"

type DockerInteractor struct {
	dockerClient *client.Client

	network string
	image   ImageInfo
}

func (interactor *DockerInteractor) ListContainers() ([]string, error) {
	result := []string{}
	ctx := context.Background()

	args := filters.NewArgs(filters.KeyValuePair{Key: "ancestor", Value: interactor.image.ImageName}, filters.KeyValuePair{Key: "status", Value: "running"})
	containers, err := interactor.dockerClient.ContainerList(ctx, types.ContainerListOptions{Filters: args})
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

	containerInfo, err := interactor.dockerClient.ContainerInspect(ctx, id)
	if err != nil {
		return result, err
	}

	binding := containerInfo.NetworkSettings.Ports[interactor.image.ImageExposedPort]
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
	if interactor.image.ImageRegistryUsername != "" {
		authConfig := registry.AuthConfig{
			Username: interactor.image.ImageRegistryUsername,
			Password: interactor.image.ImageRegisrtyPassword,
		}

		encodedConfig, err := registry.EncodeAuthConfig(authConfig)
		if err != nil {
			return "", err
		}

		pullOptions.RegistryAuth = encodedConfig
	}

	log.Printf("Pulling image %v", interactor.image.ImageName)
	out, err := interactor.dockerClient.ImagePull(ctx, interactor.image.ImageName, pullOptions)
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

	//range of ports used for bindings can be limited in
	///proc/sys/net/ipv4/ip_local_port_range
	hostConfig := container.HostConfig{}
	portBindings := []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "0"}}
	hostConfig.PortBindings = make(nat.PortMap)
	hostConfig.PortBindings[interactor.image.ImageExposedPort] = portBindings

	hostConfig.NetworkMode = container.NetworkMode(interactor.network)

	log.Println("Creating continer")
	resp, err := interactor.dockerClient.ContainerCreate(ctx, &container.Config{Image: interactor.image.ImageName}, &hostConfig, nil, nil, "")
	if err != nil {
		return "", err
	}
	log.Printf("Created container %v", resp.ID)

	err = interactor.dockerClient.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

type DockerContainerInteractorOptions struct {
	DockerNetwork string
}

func CreateDockerContainerInteractor(image ImageInfo, options DockerContainerInteractorOptions) (ContainerInteractor, error) {
	docker, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	interactor := DockerInteractor{
		dockerClient: docker,
		image:        image,
		network:      options.DockerNetwork,
	}

	return &interactor, nil
}
