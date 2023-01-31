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

const (
	RESERVE_RUNNING = 0
	RESERVE_EXITED  = 1
	RESERVE_NEW     = 2
)

type ContainerReservationArgs struct {
	reserveType int
	err         error
	panic       string
}

func TestContainerReservation(t *testing.T) {
	tests := []struct {
		name string
		args ContainerReservationArgs
		want error
	}{
		{
			name: "reserve running",
			args: ContainerReservationArgs{
				reserveType: RESERVE_RUNNING,
				err:         nil,
				panic:       "",
			},
			want: nil,
		},
		{
			name: "reserve exited",
			args: ContainerReservationArgs{
				reserveType: RESERVE_EXITED,
				err:         nil,
				panic:       "",
			},
			want: nil,
		},
		{
			name: "reserve new",
			args: ContainerReservationArgs{
				reserveType: RESERVE_NEW,
				err:         nil,
				panic:       "",
			},
			want: nil,
		},
		{
			name: "error on docker list",
			args: ContainerReservationArgs{
				reserveType: RESERVE_RUNNING,
				err:         errors.New("reserve error"),
				panic:       "",
			},
			want: errors.New("reserve error"),
		},
		{
			name: "panic on docker list",
			args: ContainerReservationArgs{
				reserveType: RESERVE_RUNNING,
				err:         nil,
				panic:       "reserve panic",
			},
			want: errors.New("reserve panic"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestID := "request1"
			containerHostname := "container"
			containerExposedPort := "3000/tcp"
			containerBindedPort := "34999"
			containerControlPort := "3000"
			externalHostname := "localhost"

			redisMock := mocks.RedisClientMock{}
			dockerMock := DockerClientMock{}
			httpMock := mocks.HTTPClientMock{}
			readCloserMock := ReadCloserMock{}

			processor := Processor{
				RedisClient:  &redisMock,
				DockerClient: &dockerMock,
				HttpClient:   &httpMock,

				ImageExposedPort: nat.Port(containerExposedPort),
				ImageControlPort: containerControlPort,
				Hostname:         externalHostname + ":",
			}

			// update request to IN_PROGRESS
			redisMock.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&redis.StatusCmd{}).Once()

			if test.args.reserveType == RESERVE_RUNNING {
				//return running conatiner
				containerArray := []types.Container{{}}
				dockerMock.On("ContainerList", mock.Anything, mock.Anything).Return(containerArray, test.args.err, test.args.panic).Once()
			} else if test.args.reserveType == RESERVE_EXITED {
				//return no running
				containerArray := []types.Container{}
				dockerMock.On("ContainerList", mock.Anything, mock.Anything).Return(containerArray, test.args.err, test.args.panic).Once()

				//return exited
				containerArray = []types.Container{{}}
				dockerMock.On("ContainerList", mock.Anything, mock.Anything).Return(containerArray, nil).Once()

				// start container
				dockerMock.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
			} else {
				//return no running
				containerArray := []types.Container{}
				dockerMock.On("ContainerList", mock.Anything, mock.Anything).Return(containerArray, test.args.err, test.args.panic).Once()

				//return no exited
				dockerMock.On("ContainerList", mock.Anything, mock.Anything).Return(containerArray, nil).Once()

				// pull image
				readCloserMock.On("Close").Return(nil).Once()
				dockerMock.On("ImagePull", mock.Anything, mock.Anything, mock.Anything).Return(&readCloserMock, nil).Once()

				// create new container
				createResponse := container.ContainerCreateCreatedBody{}
				dockerMock.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(createResponse, nil).Once()

				// start container
				dockerMock.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
			}

			// inspect to get port and hostname
			portMap := make(nat.PortMap)
			portBinding := nat.PortBinding{HostPort: containerBindedPort}
			portMap[nat.Port(containerExposedPort)] = append(portMap[nat.Port(containerExposedPort)], portBinding)

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
			assert.NoError(t, err)

			httpResponse := http.Response{StatusCode: 200}
			httpMock.On("Do", req).Return(&httpResponse, nil).Once()

			// update request to DONE
			var request common.RequestBody
			request.ID = requestID
			if test.want == nil {
				request.Status = common.DONE
				request.Server = externalHostname + ":" + containerBindedPort
				request.Container = containerHostname
			} else {
				request.Status = common.FAILED
			}

			requestJSON, err := json.Marshal(request)
			assert.NoError(t, err)
			redisMock.On("Set", mock.Anything, mock.Anything, string(requestJSON), mock.Anything).Return(&redis.StatusCmd{}).Once()

			// create initial request
			request = common.RequestBody{
				ID:     requestID,
				Status: common.CREATED,
			}

			requestJSON, err = json.Marshal(request)
			assert.NoError(t, err)

			err = processor.ProcessMessage(string(requestJSON))
			assert.Equal(t, err, test.want)

			if test.want == nil {
				readCloserMock.AssertExpectations(t)
				redisMock.AssertExpectations(t)
				dockerMock.AssertExpectations(t)
				httpMock.AssertExpectations(t)
			}
		})
	}
}
