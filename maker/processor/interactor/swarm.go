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

type SwarmInteractor struct {
	DockerClient *client.Client

	ImageRegistryUsername string
	ImageRegisrtyPassword string

	DockerNetwork    string
	ImageName        string
	ImageExposedPort nat.Port

	ConvergeVerifyCooldown int
	ConvergeVerifyRetries  int
}

func (interactor *SwarmInteractor) ListContainers() ([]string, error) {
	result := []string{}
	ctx := context.Background()

	services, err := interactor.DockerClient.ServiceList(ctx, types.ServiceListOptions{Status: true})
	if err != nil {
		return result, err
	}

	for _, service := range services {
		serviceExited := service.ServiceStatus.RunningTasks == 0
		if serviceExited {
			continue
		}

		if !strings.HasPrefix(service.Spec.TaskTemplate.ContainerSpec.Image, interactor.ImageName) {
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

	service, _, err := interactor.DockerClient.ServiceInspectWithRaw(ctx, id, types.ServiceInspectOptions{})
	if err != nil {
		return result, err
	}

	exposedPort := ""
	for _, portConfig := range service.Endpoint.Ports {
		port, err := nat.NewPort(string(portConfig.Protocol), strconv.Itoa(int(portConfig.TargetPort)))
		if err != nil {
			return result, errors.New("failed to parse port")
		}

		if port == interactor.ImageExposedPort {
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

		if network.Network.Spec.Name == interactor.DockerNetwork {
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
	if interactor.ImageRegistryUsername != "" {
		authConfig := types.AuthConfig{
			Username: interactor.ImageRegistryUsername,
			Password: interactor.ImageRegisrtyPassword,
		}

		encodedJSON, err := json.Marshal(authConfig)
		if err != nil {
			return "", err
		}

		serviceCreateOptions.EncodedRegistryAuth = base64.URLEncoding.EncodeToString(encodedJSON)
	}

	serviceSpec := swarm.ServiceSpec{}

	containerSpec := swarm.ContainerSpec{}
	containerSpec.Image = interactor.ImageName

	//range of ports used for bindings can be limited in
	///proc/sys/net/ipv4/ip_local_port_range
	portConfig := swarm.PortConfig{}
	portConfig.Protocol = swarm.PortConfigProtocol(interactor.ImageExposedPort.Proto())
	portConfig.PublishedPort = 0
	portConfig.TargetPort = uint32(interactor.ImageExposedPort.Int())

	endpointSpec := swarm.EndpointSpec{}
	endpointSpec.Ports = []swarm.PortConfig{portConfig}

	networkAttachment := swarm.NetworkAttachmentConfig{}
	networkAttachment.Target = interactor.DockerNetwork

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
	response, err := interactor.DockerClient.ServiceCreate(ctx, serviceSpec, serviceCreateOptions)
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
	tasks, err := interactor.DockerClient.TaskList(ctx, types.TaskListOptions{Filters: args})
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
