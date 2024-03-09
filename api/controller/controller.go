package controller

import (
	"log"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/st-matskevich/go-matchmaker/api/auth"
	"github.com/st-matskevich/go-matchmaker/common"
	"github.com/st-matskevich/go-matchmaker/common/data"
	"github.com/st-matskevich/go-matchmaker/common/interfaces"
)

type Controller struct {
	DataProvider data.DataProvider
	HttpClient   interfaces.HTTPClient

	ImageControlPort string
}

func (controller *Controller) HandleCreateRequest(c *fiber.Ctx) error {
	clientID := c.Locals(auth.CLIENT_ID_CTX_KEY).(string)
	if clientID == "" {
		log.Println("Got empty client ID")
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	locker := common.RequestBody{ID: clientID, Status: common.OCCUPIED}
	request, err := controller.DataProvider.Set(locker)
	if err != nil {
		log.Printf("GetClientRequest error: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	log.Printf("Got request from client %v", clientID)

	createNewRequest := false
	if request == nil || request.Status == common.FAILED {
		log.Printf("Client %v last request is failed or nil", clientID)
		createNewRequest = true
	} else if request.Status == common.CREATED || request.Status == common.IN_PROGRESS || request.Status == common.OCCUPIED {
		log.Printf("Client %v request is in progress", clientID)
		createNewRequest = false
	} else if request.Status == common.DONE {
		pending, err := controller.getReservationStatus(*request)
		if err != nil {
			//don't return, maybe just found closed container, create new request
			log.Printf("Reservation verify error: %v", err)
		}

		if err == nil && pending {
			log.Printf("Client %v reservation is OK, sending server address", clientID)
			//set back done status for future calls
			_, err = controller.DataProvider.Set(*request)
			if err != nil {
				return c.SendStatus(fiber.StatusInternalServerError)
			}

			//hostname can contain port, remove last :.* part
			parts := strings.Split(c.Hostname(), ":")
			hostname := strings.Join(parts[:len(parts)-1], ":") + ":" + request.ServerPort
			return c.Status(fiber.StatusOK).SendString(hostname)
		} else {
			log.Printf("Client %v reservation is not pending", clientID)
			createNewRequest = true
		}
	} else {
		log.Println("Found not implemented status")
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	if createNewRequest {
		err = controller.createRequest(clientID)
		if err != nil {
			log.Printf("CreateRequest error: %v", err)
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		log.Printf("Created new request for client %v", clientID)
	}

	return c.SendStatus(fiber.StatusAccepted)
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

func (controller *Controller) createRequest(clientID string) error {
	request := common.RequestBody{ID: clientID, Status: common.CREATED}
	_, err := controller.DataProvider.Set(request)
	if err != nil {
		return err
	}

	err = controller.DataProvider.ListPush(request.ID)
	if err != nil {
		return err
	}

	return nil
}
