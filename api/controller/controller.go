package controller

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sony/sonyflake"
	"github.com/st-matskevich/go-matchmaker/api/auth"
	"github.com/st-matskevich/go-matchmaker/common"
)

type Controller struct {
	idGenerator *sonyflake.Sonyflake
	redisClient *redis.Client
	httpClient  *http.Client

	imageControlPort string
}

func (controller *Controller) Init(generator *sonyflake.Sonyflake, redis *redis.Client) {
	controller.idGenerator = generator
	controller.redisClient = redis
	controller.httpClient = &http.Client{Timeout: 5 * time.Second}

	controller.imageControlPort = os.Getenv("IMAGE_CONTROL_PORT")
}

func (controller *Controller) HandleCreateRequest(c *fiber.Ctx) error {
	ctx := context.Background()
	clientID := c.Locals(auth.CLIENT_ID_CTX_KEY).(string)
	if clientID == "" {
		log.Println("Got empty client ID")
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	ok, request, err := controller.GetClientRequest(ctx, clientID)
	if err != nil {
		log.Printf("GetClientRequest error: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	log.Printf("Got request from client %v", clientID)

	createNewRequest := false
	if !ok || request.Status == common.FAILED {
		log.Printf("Client %v last request is failed", clientID)
		createNewRequest = true
	} else if request.Status == common.CREATED || request.Status == common.IN_PROGRESS || request.Status == common.OCCUPIED {
		log.Printf("Client %v request is in progress", clientID)
		createNewRequest = false
	} else if request.Status == common.DONE {
		pending, err := controller.GetReservationStatus(request)
		if err != nil {
			//don't return, maybe just found closed container, create new request
			log.Printf("Reservation verify error: %v", err)
		}

		if err == nil && pending {
			log.Printf("Client %v reservation is OK, sending server address", clientID)
			//set back done status for future calls
			err = controller.UpdateRequest(ctx, request)
			if err != nil {
				return c.SendStatus(fiber.StatusInternalServerError)
			}
			return c.Status(fiber.StatusOK).SendString(request.Server)
		} else {
			log.Printf("Client %v reservation is not pending", clientID)
			createNewRequest = true
		}
	} else {
		log.Println("Found not implemented status")
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	if createNewRequest {
		err = controller.CreateRequest(ctx, clientID)
		if err != nil {
			log.Printf("CreateRequest error: %v", err)
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		log.Printf("Created new request for client %v", clientID)
	}

	return c.SendStatus(fiber.StatusAccepted)
}

func (controller *Controller) GetClientRequest(ctx context.Context, clientID string) (bool, common.RequestBody, error) {
	result := common.RequestBody{}

	requestID, err := controller.redisClient.Get(ctx, common.GetClientKey(clientID)).Result()
	if err == redis.Nil {
		return false, result, nil
	} else if err != nil {
		return false, result, err
	}

	setArgs := redis.SetArgs{
		Get: true,
	}
	result = common.RequestBody{Status: common.OCCUPIED}
	bytes, err := json.Marshal(result)
	if err != nil {
		return false, result, err
	}

	//get request body and set as OCCUPIED
	requestJSON, err := controller.redisClient.SetArgs(ctx, common.GetRequestKey(requestID), bytes, setArgs).Result()
	if err == redis.Nil {
		return false, result, nil
	} else if err != nil {
		return false, result, err
	}

	err = json.Unmarshal([]byte(requestJSON), &result)
	if err != nil {
		return false, result, err
	}

	return true, result, nil
}

func (controller *Controller) UpdateRequest(ctx context.Context, request common.RequestBody) error {
	bytes, err := json.Marshal(request)
	if err != nil {
		return err
	}

	stringID := strconv.FormatUint(request.ID, 10)
	err = controller.redisClient.Set(ctx, common.GetRequestKey(stringID), string(bytes), 0).Err()
	if err != nil {
		return err
	}

	return nil
}

func (controller *Controller) GetReservationStatus(request common.RequestBody) (bool, error) {
	containerURL := "http://" + request.Container + ":" + controller.imageControlPort
	containerURL += "/reservation/" + strconv.FormatUint(request.ID, 10)
	resp, err := controller.httpClient.Get(containerURL)
	if err != nil {
		return false, err
	}

	return resp.StatusCode == 200, nil
}

func (controller *Controller) CreateRequest(ctx context.Context, clientID string) error {
	id, err := controller.idGenerator.NextID()
	if err != nil {
		return err
	}

	body := common.RequestBody{ID: id, Status: common.CREATED}
	bytes, err := json.Marshal(body)
	if err != nil {
		return err
	}

	stringID := strconv.FormatUint(id, 10)
	err = controller.redisClient.Set(ctx, common.GetRequestKey(stringID), string(bytes), 0).Err()
	if err != nil {
		return err
	}

	err = controller.redisClient.Set(ctx, common.GetClientKey(clientID), stringID, 0).Err()
	if err != nil {
		return err
	}

	err = controller.redisClient.LPush(ctx, common.REDIS_QUEUE_LIST_KEY, string(bytes)).Err()
	if err != nil {
		return err
	}

	return nil
}
