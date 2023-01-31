package controller

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/st-matskevich/go-matchmaker/api/auth"
	"github.com/st-matskevich/go-matchmaker/common"
	"github.com/st-matskevich/go-matchmaker/common/interfaces"
)

type Controller struct {
	RedisClient interfaces.RedisClient
	HttpClient  interfaces.HTTPClient

	ImageControlPort string
}

func (controller *Controller) HandleCreateRequest(c *fiber.Ctx) error {
	ctx := context.Background()
	clientID := c.Locals(auth.CLIENT_ID_CTX_KEY).(string)
	if clientID == "" {
		log.Println("Got empty client ID")
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	ok, request, err := controller.getClientRequest(ctx, clientID)
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
		pending, err := controller.getReservationStatus(request)
		if err != nil {
			//don't return, maybe just found closed container, create new request
			log.Printf("Reservation verify error: %v", err)
		}

		if err == nil && pending {
			log.Printf("Client %v reservation is OK, sending server address", clientID)
			//set back done status for future calls
			err = controller.updateRequest(ctx, request)
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
		err = controller.createRequest(ctx, clientID)
		if err != nil {
			log.Printf("CreateRequest error: %v", err)
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		log.Printf("Created new request for client %v", clientID)
	}

	return c.SendStatus(fiber.StatusAccepted)
}

func (controller *Controller) getClientRequest(ctx context.Context, clientID string) (bool, common.RequestBody, error) {
	setArgs := redis.SetArgs{Get: true}
	result := common.RequestBody{ID: clientID, Status: common.OCCUPIED}
	bytes, err := json.Marshal(result)
	if err != nil {
		return false, result, err
	}

	//get request body and set as OCCUPIED
	requestJSON, err := controller.RedisClient.SetArgs(ctx, clientID, bytes, setArgs).Result()
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

func (controller *Controller) updateRequest(ctx context.Context, request common.RequestBody) error {
	bytes, err := json.Marshal(request)
	if err != nil {
		return err
	}

	err = controller.RedisClient.Set(ctx, request.ID, string(bytes), 0).Err()
	if err != nil {
		return err
	}

	return nil
}

func (controller *Controller) getReservationStatus(request common.RequestBody) (bool, error) {
	containerURL := "http://" + request.Container + ":" + controller.ImageControlPort
	containerURL += "/reservation/" + request.ID

	req, err := http.NewRequest("GET", containerURL, nil)
	if err != nil {
		return false, err
	}

	resp, err := controller.HttpClient.Do(req)
	if err != nil {
		return false, err
	}

	return resp.StatusCode == 200, nil
}

func (controller *Controller) createRequest(ctx context.Context, clientID string) error {
	body := common.RequestBody{ID: clientID, Status: common.CREATED}
	bytes, err := json.Marshal(body)
	if err != nil {
		return err
	}

	err = controller.RedisClient.Set(ctx, clientID, string(bytes), 0).Err()
	if err != nil {
		return err
	}

	err = controller.RedisClient.LPush(ctx, common.REDIS_QUEUE_LIST_KEY, string(bytes)).Err()
	if err != nil {
		return err
	}

	return nil
}
