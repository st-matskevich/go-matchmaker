package processor

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/redis/go-redis/v9"
	"github.com/st-matskevich/go-matchmaker/common"
	"github.com/st-matskevich/go-matchmaker/common/mocks"
	"github.com/stretchr/testify/mock"
)

// TODO:
// - find runnin
// - find exited
// - try panic

func TestCreateContainer(t *testing.T) {
	//expected values
	requestID := "request1"
	containerHostname := "container"
	containerExposedPort := "3000/tcp"
	containerBindedPort := "34999"
	containerControlPort := "3000"
	externalHostname := "localhost"

	redisMock := mocks.RedisClientMock{}
	dockerMock := DockerClientMock{}
	httpMock := mocks.HTTPClientMock{}

	processor := Processor{
		RedisClient:  &redisMock,
		DockerClient: &dockerMock,
		HttpClient:   &httpMock,

		ImageExposedPort: nat.Port(containerExposedPort),
		ImageControlPort: containerControlPort,
		Hostname:         externalHostname + ":",
	}

	// update request to IN_PROGRESS
	redisStatus := redis.StatusCmd{}
	redisStatus.SetErr(nil)
	redisMock.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&redisStatus).Once()

	// list running containers, return empty array
	dockerMock.On("ContainerList", mock.Anything, mock.Anything).Return([]types.Container{}, nil).Once()

	// list exited containers, return empty array
	dockerMock.On("ContainerList", mock.Anything, mock.Anything).Return([]types.Container{}, nil).Once()

	// pull image
	readCloserMock := ReadCloserMock{}
	readCloserMock.On("Close").Return(nil).Once()
	dockerMock.On("ImagePull", mock.Anything, mock.Anything, mock.Anything).Return(&readCloserMock, nil).Once()

	// create new container
	createResponse := container.ContainerCreateCreatedBody{}
	dockerMock.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(createResponse, nil).Once()

	// start container
	dockerMock.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	// inspect to get port and hostname
	portMap := make(nat.PortMap)
	portBinding := nat.PortBinding{HostPort: containerBindedPort}
	portMap[processor.ImageExposedPort] = append(portMap[processor.ImageExposedPort], portBinding)

	inspectResponse := types.ContainerJSON{}
	inspectResponse.NetworkSettings = &types.NetworkSettings{}
	inspectResponse.NetworkSettings.Ports = portMap

	inspectResponse.Config = &container.Config{}
	inspectResponse.Config.Hostname = containerHostname

	dockerMock.On("ContainerInspect", mock.Anything, mock.Anything).Return(inspectResponse, nil).Once()

	// container reservation request
	containerURL := "http://" + containerHostname + ":" + containerControlPort
	containerURL += "/reservation/" + requestID
	req, err := http.NewRequest("POST", containerURL, nil)
	if err != nil {
		t.Fatalf("NewRequest error: %v", err)
	}

	httpResponse := http.Response{}
	httpResponse.StatusCode = 200
	httpMock.On("Do", req).Return(&httpResponse, nil).Once()

	// update request to DONE
	request := common.RequestBody{
		ID:        requestID,
		Status:    common.DONE,
		Server:    externalHostname + ":" + containerBindedPort,
		Container: containerHostname,
	}

	bytes, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	redisStatus = redis.StatusCmd{}
	redisStatus.SetErr(nil)
	redisMock.On("Set", mock.Anything, mock.Anything, string(bytes), mock.Anything).Return(&redisStatus).Once()

	// create initial request
	request = common.RequestBody{
		ID:     requestID,
		Status: common.CREATED,
	}

	bytes, err = json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	err = processor.ProcessMessage(string(bytes))
	if err != nil {
		t.Fatalf("got error from processor: %v", err)
	}

	readCloserMock.AssertExpectations(t)
	redisMock.AssertExpectations(t)
	dockerMock.AssertExpectations(t)
	httpMock.AssertExpectations(t)
}

func TestWriteRequestOnError(t *testing.T) {
	//expected values
	requestID := "request1"
	redisMock := mocks.RedisClientMock{}
	dockerMock := DockerClientMock{}
	httpMock := mocks.HTTPClientMock{}

	processor := Processor{
		RedisClient:  &redisMock,
		DockerClient: &dockerMock,
		HttpClient:   &httpMock,
	}

	// throw error on IN_PROGRESS write
	redisError := errors.New("redis error")
	redisStatusError := redis.StatusCmd{}
	redisStatusError.SetErr(redisError)
	redisMock.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&redisStatusError).Once()

	request := common.RequestBody{
		ID:     requestID,
		Status: common.FAILED,
	}

	bytes, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// expect FAILED write
	redisStatus := redis.StatusCmd{}
	redisStatus.SetErr(nil)
	redisMock.On("Set", mock.Anything, mock.Anything, string(bytes), mock.Anything).Return(&redisStatus).Once()

	request = common.RequestBody{
		ID:     requestID,
		Status: common.CREATED,
	}

	bytes, err = json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	err = processor.ProcessMessage(string(bytes))
	if err != redisError {
		t.Fatalf("got error from processor: %v", err)
	}

	redisMock.AssertExpectations(t)
	dockerMock.AssertExpectations(t)
	httpMock.AssertExpectations(t)
}
