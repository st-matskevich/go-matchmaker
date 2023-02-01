package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/st-matskevich/go-matchmaker/api/auth"
	"github.com/st-matskevich/go-matchmaker/common"
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
					ID:        "client1",
					Status:    common.DONE,
					Container: "container1",
					Server:    "server1:45677",
				},
			},
			want: RequestHandlingWant{
				code: fiber.StatusOK,
				body: "server1:45677",
			},
		},
		{
			name: "request DONE reservation not pending",
			args: RequestHandlingArgs{
				clientID:        "client1",
				reservationCode: fiber.StatusNotFound,
				request: &common.RequestBody{
					ID:        "client1",
					Status:    common.DONE,
					Container: "container1",
					Server:    "server1:45677",
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

			redisMock := mocks.RedisClientMock{}
			httpMock := mocks.HTTPClientMock{}

			controller := Controller{
				RedisClient:      &redisMock,
				HttpClient:       &httpMock,
				ImageControlPort: containerControlPort,
			}

			redisStatus := &redis.StatusCmd{}
			if test.args.request != nil {
				requestJSON, err := json.Marshal(test.args.request)
				assert.Nil(t, err)
				redisStatus.SetVal(string(requestJSON))
			} else {
				redisStatus.SetErr(redis.Nil)
			}

			//set with get on request
			if test.args.clientID != "" {
				redisMock.On("SetArgs", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(redisStatus).Once()
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
					redisMock.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&redis.StatusCmd{}).Once()
				} else {
					//expect new request
					redisMock.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&redis.StatusCmd{}).Once()
					redisMock.On("LPush", mock.Anything, mock.Anything, mock.Anything).Return(&redis.IntCmd{}).Once()
				}
			}

			if test.args.request == nil || test.args.request.Status == common.FAILED {
				//expect new request
				redisMock.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&redis.StatusCmd{}).Once()
				redisMock.On("LPush", mock.Anything, mock.Anything, mock.Anything).Return(&redis.IntCmd{}).Once()
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

			redisMock.AssertExpectations(t)
			httpMock.AssertExpectations(t)
		})
	}
}
