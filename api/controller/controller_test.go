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
	body *string
}

type RequestHandlingArgs struct {
	clientID string
	request  *common.RequestBody
}

func TestRequestHandling(t *testing.T) {
	tests := []struct {
		name string
		args RequestHandlingArgs
		want RequestHandlingWant
	}{
		{
			name: "request NONE",
			args: RequestHandlingArgs{
				clientID: "client1",
				request:  nil,
			},
			want: RequestHandlingWant{
				code: fiber.StatusAccepted,
				body: nil,
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
				body: nil,
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
			redisMock.On("SetArgs", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(redisStatus).Once()

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

			response, err := app.Test(httpRequest, -1)
			assert.NoError(t, err)
			defer response.Body.Close()

			assert.Equal(t, response.StatusCode, test.want.code)
			if test.want.body != nil {
				bodyString, err := io.ReadAll(response.Body)
				assert.NoError(t, err)
				assert.Equal(t, bodyString, *test.want.body)
			}

			redisMock.AssertExpectations(t)
			httpMock.AssertExpectations(t)
		})
	}
}
