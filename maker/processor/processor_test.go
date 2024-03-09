package processor

import (
	"errors"
	"net/http"
	"testing"

	"github.com/st-matskevich/go-matchmaker/common"
	"github.com/st-matskevich/go-matchmaker/common/data"
	"github.com/st-matskevich/go-matchmaker/common/web"
	"github.com/st-matskevich/go-matchmaker/maker/processor/interactor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const (
	RESERVE_RUNNING = 0
	RESERVE_NEW     = 1
)

type ContainerReservationArgs struct {
	reserveType      int
	registryUsername string
	registryPassword string
	err              error
	panic            string
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
			name: "reserve new",
			args: ContainerReservationArgs{
				reserveType: RESERVE_NEW,
				err:         nil,
				panic:       "",
			},
			want: nil,
		},
		{
			name: "reserve new with auth",
			args: ContainerReservationArgs{
				reserveType:      RESERVE_NEW,
				registryUsername: "user",
				registryPassword: "password",
				err:              nil,
				panic:            "",
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
			containerBindedPort := "34999"
			containerControlPort := "3000"

			dataProvider := data.MockDataProvider{}
			dockerMock := interactor.MockInteractor{}
			httpMock := web.HTTPClientMock{}

			processor := Processor{
				DataProvider: &dataProvider,
				DockerClient: &dockerMock,
				HttpClient:   &httpMock,

				ImageControlPort: containerControlPort,
			}

			var request common.RequestBody
			request.ID = requestID
			request.Status = common.CREATED

			// update request to IN_PROGRESS
			dataProvider.On("Set", mock.Anything).Return(&request, nil).Once()

			if test.args.reserveType == RESERVE_RUNNING {
				containerArray := []string{""}
				dockerMock.On("ListContainers").Return(containerArray, test.args.err, test.args.panic).Once()
			} else {
				containerArray := []string{}
				dockerMock.On("ListContainers").Return(containerArray, test.args.err, test.args.panic).Once()
				dockerMock.On("CreateContainer").Return("", nil).Once()
			}

			inspectResponse := interactor.ContainerInfo{}
			inspectResponse.Address = containerHostname
			inspectResponse.ExposedPort = containerBindedPort
			dockerMock.On("InspectContainer", mock.Anything).Return(inspectResponse, nil).Once()

			// container reservation request
			containerURL := "http://" + containerHostname + ":" + containerControlPort
			containerURL += "/reservation/" + requestID
			req, err := http.NewRequest("POST", containerURL, nil)
			assert.NoError(t, err)

			httpResponse := http.Response{StatusCode: 200}
			httpMock.On("Do", req).Return(&httpResponse, nil).Once()

			// update request to DONE
			dataProvider.On("Set", mock.Anything).Return(nil, nil).Once()

			// create initial request
			err = processor.processMessage(requestID)
			assert.Equal(t, test.want, err)

			if test.want == nil {
				dataProvider.AssertExpectations(t)
				dockerMock.AssertExpectations(t)
				httpMock.AssertExpectations(t)
			}
		})
	}
}
