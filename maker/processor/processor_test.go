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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func createContainerInspectResponse(hostname string, exposedPort string, bindedPort string) types.ContainerJSON {
	portMap := make(nat.PortMap)
	portBinding := nat.PortBinding{HostPort: bindedPort}
	portMap[nat.Port(exposedPort)] = append(portMap[nat.Port(exposedPort)], portBinding)

	inspectResponse := types.ContainerJSON{}
	inspectResponse.NetworkSettings = &types.NetworkSettings{}
	inspectResponse.NetworkSettings.Ports = portMap

	inspectResponse.Config = &container.Config{}
	inspectResponse.Config.Hostname = hostname

	return inspectResponse
}

func createReservationRequest(hostname string, controlPort string, requestID string) (*http.Request, error) {
	containerURL := "http://" + hostname + ":" + controlPort
	containerURL += "/reservation/" + requestID
	return http.NewRequest("POST", containerURL, nil)
}

func addRedisSetExpectation(t *testing.T, redisMock *mocks.RedisClientMock, request *common.RequestBody, err error) {
	data := mock.Anything
	if request != nil {
		bytes, err := json.Marshal(request)
		assert.Nil(t, err)
		data = string(bytes)
	}

	redisStatus := redis.StatusCmd{}
	redisStatus.SetErr(err)
	redisMock.On("Set", mock.Anything, mock.Anything, data, mock.Anything).Return(&redisStatus).Once()
}

func TestReserveRunningContainer(t *testing.T) {
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
	addRedisSetExpectation(t, &redisMock, nil, nil)

	// list running containers
	listArray := []types.Container{{}}
	dockerMock.On("ContainerList", mock.Anything, mock.Anything).Return(listArray, nil).Once()

	// inspect to get port and hostname
	inspectResponse := createContainerInspectResponse(containerHostname, containerExposedPort, containerBindedPort)
	dockerMock.On("ContainerInspect", mock.Anything, mock.Anything).Return(inspectResponse, nil).Once()

	// container reservation request
	req, err := createReservationRequest(containerHostname, containerControlPort, requestID)
	assert.Nil(t, err)

	httpResponse := http.Response{StatusCode: 200}
	httpMock.On("Do", req).Return(&httpResponse, nil).Once()

	// update request to DONE
	request := common.RequestBody{
		ID:        requestID,
		Status:    common.DONE,
		Server:    externalHostname + ":" + containerBindedPort,
		Container: containerHostname,
	}

	addRedisSetExpectation(t, &redisMock, &request, nil)

	// create initial request
	request = common.RequestBody{
		ID:     requestID,
		Status: common.CREATED,
	}

	bytes, err := json.Marshal(request)
	assert.Nil(t, err)

	err = processor.ProcessMessage(string(bytes))
	assert.Nil(t, err)

	redisMock.AssertExpectations(t)
	dockerMock.AssertExpectations(t)
	httpMock.AssertExpectations(t)
}

func TestStartExitedContainer(t *testing.T) {
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
	addRedisSetExpectation(t, &redisMock, nil, nil)

	// list running containers, return empty array
	dockerMock.On("ContainerList", mock.Anything, mock.Anything).Return([]types.Container{}, nil).Once()

	// list exited containers, return empty array
	listArray := []types.Container{{}}
	dockerMock.On("ContainerList", mock.Anything, mock.Anything).Return(listArray, nil).Once()

	// start container
	dockerMock.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	// inspect to get port and hostname
	inspectResponse := createContainerInspectResponse(containerHostname, containerExposedPort, containerBindedPort)
	dockerMock.On("ContainerInspect", mock.Anything, mock.Anything).Return(inspectResponse, nil).Once()

	// container reservation request
	req, err := createReservationRequest(containerHostname, containerControlPort, requestID)
	assert.Nil(t, err)

	httpResponse := http.Response{StatusCode: 200}
	httpMock.On("Do", req).Return(&httpResponse, nil).Once()

	// update request to DONE
	request := common.RequestBody{
		ID:        requestID,
		Status:    common.DONE,
		Server:    externalHostname + ":" + containerBindedPort,
		Container: containerHostname,
	}

	addRedisSetExpectation(t, &redisMock, &request, nil)

	// create initial request
	request = common.RequestBody{
		ID:     requestID,
		Status: common.CREATED,
	}

	bytes, err := json.Marshal(request)
	assert.Nil(t, err)

	err = processor.ProcessMessage(string(bytes))
	assert.Nil(t, err)

	redisMock.AssertExpectations(t)
	dockerMock.AssertExpectations(t)
	httpMock.AssertExpectations(t)
}

func TestCreateNewContainer(t *testing.T) {
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
	addRedisSetExpectation(t, &redisMock, nil, nil)

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
	inspectResponse := createContainerInspectResponse(containerHostname, containerExposedPort, containerBindedPort)
	dockerMock.On("ContainerInspect", mock.Anything, mock.Anything).Return(inspectResponse, nil).Once()

	// container reservation request
	req, err := createReservationRequest(containerHostname, containerControlPort, requestID)
	assert.Nil(t, err)

	httpResponse := http.Response{StatusCode: 200}
	httpMock.On("Do", req).Return(&httpResponse, nil).Once()

	// update request to DONE
	request := common.RequestBody{
		ID:        requestID,
		Status:    common.DONE,
		Server:    externalHostname + ":" + containerBindedPort,
		Container: containerHostname,
	}

	addRedisSetExpectation(t, &redisMock, &request, nil)

	// create initial request
	request = common.RequestBody{
		ID:     requestID,
		Status: common.CREATED,
	}

	bytes, err := json.Marshal(request)
	assert.Nil(t, err)

	err = processor.ProcessMessage(string(bytes))
	assert.Nil(t, err)

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
	addRedisSetExpectation(t, &redisMock, nil, redisError)

	request := common.RequestBody{
		ID:     requestID,
		Status: common.FAILED,
	}

	addRedisSetExpectation(t, &redisMock, &request, nil)

	request = common.RequestBody{
		ID:     requestID,
		Status: common.CREATED,
	}

	bytes, err := json.Marshal(request)
	assert.Nil(t, err)

	err = processor.ProcessMessage(string(bytes))
	assert.EqualError(t, err, redisError.Error())

	redisMock.AssertExpectations(t)
	dockerMock.AssertExpectations(t)
	httpMock.AssertExpectations(t)
}

func TestWriteRequestOnPanic(t *testing.T) {
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
	addRedisSetExpectation(t, &redisMock, nil, nil)

	// list running containers, return empty array
	panicString := "docker panic"
	dockerMock.On("ContainerList", mock.Anything, mock.Anything).Return([]types.Container{}, nil, panicString).Once()

	request := common.RequestBody{
		ID:     requestID,
		Status: common.FAILED,
	}

	addRedisSetExpectation(t, &redisMock, &request, nil)

	request = common.RequestBody{
		ID:     requestID,
		Status: common.CREATED,
	}

	bytes, err := json.Marshal(request)
	assert.Nil(t, err)

	err = processor.ProcessMessage(string(bytes))
	assert.EqualError(t, err, panicString)

	redisMock.AssertExpectations(t)
	dockerMock.AssertExpectations(t)
	httpMock.AssertExpectations(t)
}
