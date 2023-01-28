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
	"github.com/st-matskevich/go-matchmaker/api/auth"
	"github.com/st-matskevich/go-matchmaker/common"
)

type Controller struct {
	redisClient *redis.Client
	httpClient  *http.Client

	imageControlPort string
}

func (controller *Controller) Init(redis *redis.Client) error {
	controller.redisClient = redis

	timeoutString := os.Getenv("RESERVATION_TIMEOUT")
	reservationTimeout, err := strconv.Atoi(timeoutString)
	if err != nil {
		return err
	}
	controller.httpClient = &http.Client{Timeout: time.Duration(reservationTimeout) * time.Millisecond}

	controller.imageControlPort = os.Getenv("IMAGE_CONTROL_PORT")

	return nil
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
		log.Printf("Client %v last request is failed or nil", clientID)
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

	setArgs := redis.SetArgs{
		Get: true,
	}
	result = common.RequestBody{ID: clientID, Status: common.OCCUPIED}
	bytes, err := json.Marshal(result)
	if err != nil {
		return false, result, err
	}

	//get request body and set as OCCUPIED
	requestJSON, err := controller.redisClient.SetArgs(ctx, clientID, bytes, setArgs).Result()
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

	err = controller.redisClient.Set(ctx, request.ID, string(bytes), 0).Err()
	if err != nil {
		return err
	}

	return nil
}

func (controller *Controller) GetReservationStatus(request common.RequestBody) (bool, error) {
	containerURL := "http://" + request.Container + ":" + controller.imageControlPort
	containerURL += "/reservation/" + request.ID
	resp, err := controller.httpClient.Get(containerURL)
	if err != nil {
		return false, err
	}

	return resp.StatusCode == 200, nil
}

func (controller *Controller) CreateRequest(ctx context.Context, clientID string) error {
	body := common.RequestBody{ID: clientID, Status: common.CREATED}
	bytes, err := json.Marshal(body)
	if err != nil {
		return err
	}

	err = controller.redisClient.Set(ctx, clientID, string(bytes), 0).Err()
	if err != nil {
		return err
	}

	err = controller.redisClient.LPush(ctx, common.REDIS_QUEUE_LIST_KEY, string(bytes)).Err()
	if err != nil {
		return err
	}

	return nil
}