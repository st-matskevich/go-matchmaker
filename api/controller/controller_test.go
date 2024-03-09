package controller

import (
	"io"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/st-matskevich/go-matchmaker/api/auth"
	"github.com/st-matskevich/go-matchmaker/common"
	"github.com/st-matskevich/go-matchmaker/common/data"
	"github.com/st-matskevich/go-matchmaker/common/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type RequestHandlingWant struct {
	code int
	body string
}

type RequestHandlingArgs struct {
	clientID        string
	request         *common.RequestBody
	reservationCode int
}

func TestRequestHandling(t *testing.T) {
	tests := []struct {
		name string
		args RequestHandlingArgs
		want RequestHandlingWant
	}{
		{
			name: "empty client",
			args: RequestHandlingArgs{
				clientID: "",
				request: &common.RequestBody{
					ID:     "client1",
					Status: common.CREATED,
				},
			},
			want: RequestHandlingWant{
				code: fiber.StatusInternalServerError,
				body: "",
			},
		},
		{
			name: "request NONE",
			args: RequestHandlingArgs{
				clientID: "client1",
				request:  nil,
			},
			want: RequestHandlingWant{
				code: fiber.StatusAccepted,
				body: "",
			},
		},
		{
			name: "request CREATED",
			args: RequestHandlingArgs{
				clientID: "client1",
				request: &common.RequestBody{
					ID:     "client1",
					Status: common.CREATED,
				},
			},
			want: RequestHandlingWant{
				code: fiber.StatusAccepted,
				body: "",
			},
		},
		{
			name: "request IN_PROGRESS",
			args: RequestHandlingArgs{
				clientID: "client1",
				request: &common.RequestBody{
					ID:     "client1",
					Status: common.IN_PROGRESS,
				},
			},
			want: RequestHandlingWant{
				code: fiber.StatusAccepted,
				body: "",
			},
		},
		{
			name: "request OCCUPIED",
			args: RequestHandlingArgs{
				clientID: "client1",
				request: &common.RequestBody{
					ID:     "client1",
					Status: common.OCCUPIED,
				},
			},
			want: RequestHandlingWant{
				code: fiber.StatusAccepted,
				body: "",
			},
		},
		{
			name: "request FAILED",
			args: RequestHandlingArgs{
				clientID: "client1",
				request: &common.RequestBody{
					ID:     "client1",
					Status: common.FAILED,
				},
			},
			want: RequestHandlingWant{
				code: fiber.StatusAccepted,
				body: "",
			},
		},
		{
			name: "request DONE reservation pending",
			args: RequestHandlingArgs{
				clientID:        "client1",
				reservationCode: fiber.StatusOK,
				request: &common.RequestBody{
					ID:         "client1",
					Status:     common.DONE,
					Container:  "container1",
					ServerPort: "45677",
				},
			},
			want: RequestHandlingWant{
				code: fiber.StatusOK,
				//no hostname in test mode
				body: ":45677",
			},
		},
		{
			name: "request DONE reservation not pending",
			args: RequestHandlingArgs{
				clientID:        "client1",
				reservationCode: fiber.StatusNotFound,
				request: &common.RequestBody{
					ID:         "client1",
					Status:     common.DONE,
					Container:  "container1",
					ServerPort: "45677",
				},
			},
			want: RequestHandlingWant{
				code: fiber.StatusAccepted,
				body: "",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			containerControlPort := "3000"

			dataProvider := data.MockDataProvider{}
			httpMock := mocks.HTTPClientMock{}

			controller := Controller{
				DataProvider:     &dataProvider,
				HttpClient:       &httpMock,
				ImageControlPort: containerControlPort,
			}

			//set with get on request
			if test.args.clientID != "" {
				dataProvider.On("Set", mock.Anything).Return(test.args.request, nil).Once()
			}

			if test.args.request != nil && test.args.request.Status == common.DONE {
				//expect reservation request
				containerURL := "http://" + test.args.request.Container + ":" + containerControlPort
				containerURL += "/reservation/" + test.args.clientID
				req, err := http.NewRequest("GET", containerURL, nil)
				assert.NoError(t, err)

				httpResponse := http.Response{StatusCode: test.args.reservationCode}
				httpMock.On("Do", req).Return(&httpResponse, nil).Once()

				if test.args.reservationCode == fiber.StatusOK {
					//expect request update
					dataProvider.On("Set", mock.Anything).Return(nil, nil).Once()
				} else {
					//expect new request
					dataProvider.On("Set", mock.Anything).Return(nil, nil).Once()
					dataProvider.On("ListPush", mock.Anything).Return(nil).Once()
				}
			}

			if test.args.request == nil || test.args.request.Status == common.FAILED {
				//expect new request
				dataProvider.On("Set", mock.Anything).Return(nil, nil).Once()
				dataProvider.On("ListPush", mock.Anything).Return(nil).Once()
			}

			app := fiber.New()
			app.Post("/request", func(c *fiber.Ctx) error {
				c.Locals(auth.CLIENT_ID_CTX_KEY, test.args.clientID)
				return controller.HandleCreateRequest(c)
			})

			httpRequest, err := http.NewRequest("POST", "/request", nil)
			assert.NoError(t, err)

			response, err := app.Test(httpRequest)
			assert.NoError(t, err)
			defer response.Body.Close()

			assert.Equal(t, test.want.code, response.StatusCode)
			if test.want.body != "" {
				bodyBytes, err := io.ReadAll(response.Body)
				assert.NoError(t, err)
				assert.Equal(t, test.want.body, string(bodyBytes))
			}

			dataProvider.AssertExpectations(t)
			httpMock.AssertExpectations(t)
		})
	}
}
