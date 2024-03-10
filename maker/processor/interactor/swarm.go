package interactor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const SWARM_INTERACTOR = "swarm"

type SwarmInteractor struct {
	dockerClient *client.Client

	image                  ImageInfo
	network                string
	ConvergeVerifyCooldown int
	ConvergeVerifyRetries  int
}

func (interactor *SwarmInteractor) ListContainers() ([]string, error) {
	result := []string{}
	ctx := context.Background()

	services, err := interactor.dockerClient.ServiceList(ctx, types.ServiceListOptions{Status: true})
	if err != nil {
		return result, err
	}

	for _, service := range services {
		serviceExited := service.ServiceStatus.RunningTasks == 0
		if serviceExited {
			continue
		}

		if !strings.HasPrefix(service.Spec.TaskTemplate.ContainerSpec.Image, interactor.image.ImageName) {
			continue
		}

		result = append(result, service.ID)
	}

	return result, nil
}

func (interactor *SwarmInteractor) InspectContainer(id string) (ContainerInfo, error) {
	result := ContainerInfo{}
	ctx := context.Background()

	task, err := interactor.getServiceTask(id)
	if err != nil {
		return result, err
	}

	if task == nil {
		return result, errors.New("service has no tasks")
	}

	if task.Status.ContainerStatus == nil {
		return result, errors.New("task doesn't contain container status")
	}

	service, _, err := interactor.dockerClient.ServiceInspectWithRaw(ctx, id, types.ServiceInspectOptions{})
	if err != nil {
		return result, err
	}

	exposedPort := ""
	for _, portConfig := range service.Endpoint.Ports {
		port, err := nat.NewPort(string(portConfig.Protocol), strconv.Itoa(int(portConfig.TargetPort)))
		if err != nil {
			return result, errors.New("failed to parse port")
		}

		if port == interactor.image.ImageExposedPort {
			exposedPort = strconv.Itoa(int(portConfig.PublishedPort))
		}
	}

	if exposedPort == "" {
		return result, errors.New("no binding found for specified IMAGE_EXPOSE_PORT")
	}

	containerIP := ""
	for _, network := range task.NetworksAttachments {
		if len(network.Addresses) < 1 {
			continue
		}

		if network.Network.Spec.Name == interactor.network {
			parsedIP, err := netip.ParsePrefix(network.Addresses[0])
			if err != nil {
				return result, err
			}

			containerIP = parsedIP.Addr().String()
			break
		}
	}

	if containerIP == "" {
		return result, errors.New("specified service have no assigned IP on DOCKER_NETWORK")
	}

	result.Address = containerIP
	result.ExposedPort = exposedPort
	return result, nil
}

func (interactor *SwarmInteractor) CreateContainer() (string, error) {
	ctx := context.Background()
	serviceCreateOptions := types.ServiceCreateOptions{}
	if interactor.image.ImageRegistryUsername != "" {
		authConfig := types.AuthConfig{
			Username: interactor.image.ImageRegistryUsername,
			Password: interactor.image.ImageRegisrtyPassword,
		}

		encodedJSON, err := json.Marshal(authConfig)
		if err != nil {
			return "", err
		}

		serviceCreateOptions.EncodedRegistryAuth = base64.URLEncoding.EncodeToString(encodedJSON)
	}

	serviceSpec := swarm.ServiceSpec{}

	containerSpec := swarm.ContainerSpec{}
	containerSpec.Image = interactor.image.ImageName

	//range of ports used for bindings can be limited in
	///proc/sys/net/ipv4/ip_local_port_range
	portConfig := swarm.PortConfig{}
	portConfig.Protocol = swarm.PortConfigProtocol(interactor.image.ImageExposedPort.Proto())
	portConfig.PublishedPort = 0
	portConfig.TargetPort = uint32(interactor.image.ImageExposedPort.Int())

	endpointSpec := swarm.EndpointSpec{}
	endpointSpec.Ports = []swarm.PortConfig{portConfig}

	networkAttachment := swarm.NetworkAttachmentConfig{}
	networkAttachment.Target = interactor.network

	//TODO: maybe let docker handle service restarts?
	restartPolicy := swarm.RestartPolicy{}
	restartPolicy.Condition = swarm.RestartPolicyConditionNone

	replicas := uint64(1)
	serviceSpec.Mode.Replicated = &swarm.ReplicatedService{Replicas: &replicas}
	serviceSpec.TaskTemplate.ContainerSpec = &containerSpec
	serviceSpec.TaskTemplate.RestartPolicy = &restartPolicy
	serviceSpec.TaskTemplate.Networks = []swarm.NetworkAttachmentConfig{networkAttachment}
	serviceSpec.EndpointSpec = &endpointSpec

	log.Println("Creating service")
	response, err := interactor.dockerClient.ServiceCreate(ctx, serviceSpec, serviceCreateOptions)
	if err != nil {
		return "", err
	}

	//wait for converge
	retriesCounter := 0
	for {
		task, err := interactor.getServiceTask(response.ID)
		if err != nil {
			return "", err
		}

		if task == nil {
			continue
		}

		retriesCounter++
		if task.Status.ContainerStatus != nil {
			break
		} else if retriesCounter >= interactor.ConvergeVerifyRetries {
			return "", errors.New("could not receive service status")
		} else {
			time.Sleep(time.Duration(interactor.ConvergeVerifyCooldown) * time.Millisecond)
		}
	}
	log.Printf("Created service %v", response.ID)

	return response.ID, nil
}

func (interactor *SwarmInteractor) getServiceTask(id string) (*swarm.Task, error) {
	ctx := context.Background()
	args := filters.NewArgs(filters.KeyValuePair{Key: "service", Value: id})
	tasks, err := interactor.dockerClient.TaskList(ctx, types.TaskListOptions{Filters: args})
	if err != nil {
		return nil, err
	}

	tasksCount := len(tasks)
	if tasksCount > 1 {
		return nil, errors.New("expected only one task, but got " + strconv.Itoa(tasksCount))
	} else if tasksCount < 1 {
		return nil, nil
	}

	return &tasks[0], nil
}

type SwarmContainerInteractorOptions struct {
	DockerNetwork          string
	ConvergeVerifyCooldown int
	ConvergeVerifyRetries  int
}

func CreateSwarmContainerInteractor(image ImageInfo, options SwarmContainerInteractorOptions) (ContainerInteractor, error) {
	docker, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	interactor := SwarmInteractor{
		dockerClient:           docker,
		image:                  image,
		network:                options.DockerNetwork,
		ConvergeVerifyCooldown: options.ConvergeVerifyCooldown,
		ConvergeVerifyRetries:  options.ConvergeVerifyRetries,
	}

	return &interactor, nil
}
