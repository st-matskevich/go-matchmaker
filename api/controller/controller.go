package controller

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/go-redis/redis"
	"github.com/gofiber/fiber/v2"
	"github.com/sony/sonyflake"
	"github.com/st-matskevich/go-matchmaker/api/auth"
	"github.com/st-matskevich/go-matchmaker/common"
)

type Controller struct {
	idGenerator *sonyflake.Sonyflake
	redisClient *redis.Client

	imageControlPort string
}

func (controller *Controller) Init(generator *sonyflake.Sonyflake, redis *redis.Client) {
	controller.idGenerator = generator
	controller.redisClient = redis

	controller.imageControlPort = os.Getenv("IMAGE_CONTROL_PORT")
}

func (controller *Controller) HandleCreateRequest(c *fiber.Ctx) error {
	clientID := c.Locals(auth.CLIENT_ID_CTX_KEY).(string)
	if clientID == "" {
		log.Println("Got empty client ID")
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	ok, request, err := controller.GetClientRequest(clientID)
	if err != nil {
		log.Printf("GetClientRequest error: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	log.Printf("Got request from client %v", clientID)

	createNewRequest := false
	if !ok || request.Status == common.FAILED {
		log.Println("Last client request failed")
		createNewRequest = true
	} else if request.Status == common.CREATED || request.Status == common.IN_PROGRESS {
		log.Println("Client request is in progress")
		createNewRequest = false
	} else if request.Status == common.DONE {
		pending, err := controller.GetReservationStatus(request)
		if err != nil {
			//don't return, maybe just found closed container, create new request
			log.Printf("Reservation verify error: %v", err)
		}

		if err == nil && pending {
			log.Println("Client reservation is OK, sending server address")
			return c.Status(fiber.StatusOK).SendString(request.Server)
		} else {
			log.Println("Client reservation is not pending")
			createNewRequest = true
		}
	} else {
		log.Println("Found not implemented status")
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	if createNewRequest {
		err = controller.CreateRequest(clientID)
		if err != nil {
			log.Printf("CreateRequest error: %v", err)
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		log.Printf("Created new request for client %v", clientID)
	}

	return c.SendStatus(fiber.StatusAccepted)
}

func (controller *Controller) GetClientRequest(clientID string) (bool, common.RequestBody, error) {
	result := common.RequestBody{}
	count, err := controller.redisClient.Exists(common.GetClientKey(clientID)).Result()
	if err != nil {
		return false, result, err
	}

	if count < 1 {
		return false, result, nil
	}

	requestID, err := controller.redisClient.Get(common.GetClientKey(clientID)).Result()
	if err != nil {
		return false, result, err
	}

	count, err = controller.redisClient.Exists(common.GetRequestKey(requestID)).Result()
	if err != nil {
		return false, result, err
	}

	if count < 1 {
		return false, result, nil
	}

	requestJSON, err := controller.redisClient.Get(common.GetRequestKey(requestID)).Result()
	if err != nil {
		return false, result, err
	}

	err = json.Unmarshal([]byte(requestJSON), &result)
	if err != nil {
		return false, result, err
	}

	return true, result, nil
}

func (controller *Controller) GetReservationStatus(request common.RequestBody) (bool, error) {
	containerURL := "http://" + request.Container + ":" + controller.imageControlPort
	containerURL += "/reservation/" + strconv.FormatUint(request.ID, 10)
	resp, err := http.Get(containerURL)
	if err != nil {
		return false, err
	}

	return resp.StatusCode == 200, nil
}

func (controller *Controller) CreateRequest(clientID string) error {
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
	err = controller.redisClient.Set(common.GetRequestKey(stringID), string(bytes), 0).Err()
	if err != nil {
		return err
	}

	err = controller.redisClient.Set(common.GetClientKey(clientID), stringID, 0).Err()
	if err != nil {
		return err
	}

	err = controller.redisClient.LPush(common.REDIS_QUEUE_LIST_KEY, string(bytes)).Err()
	if err != nil {
		return err
	}

	return nil
}
