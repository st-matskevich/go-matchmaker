package processor

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/st-matskevich/go-matchmaker/common"
	"github.com/st-matskevich/go-matchmaker/common/interfaces"
	"github.com/st-matskevich/go-matchmaker/maker/processor/interactor"
)

type Processor struct {
	RedisClient  interfaces.RedisClient
	DockerClient interactor.ContainerInteractor
	HttpClient   interfaces.HTTPClient
	creatorMutex sync.Mutex

	ImageControlPort string

	LookupCooldownMillisecond int

	ReservationRetries  int
	ReservationCooldown int
}

func (processor *Processor) fillRequestWithContainerInfo(request *common.RequestBody, info *interactor.ContainerInfo) {
	request.Container = info.Address
	request.ServerPort = info.ExposedPort
}

func (processor *Processor) writeRequest(ctx context.Context, req *common.RequestBody) error {
	bytes, err := json.Marshal(req)
	if err != nil {
		return err
	}

	err = processor.RedisClient.Set(ctx, req.ID, string(bytes), 0).Err()
	if err != nil {
		return err
	}

	return nil
}

func (processor *Processor) ProcessMessage(message string) (rerr error) {
	var request common.RequestBody
	ctx := context.Background()
	err := json.Unmarshal([]byte(message), &request)
	if err != nil {
		return err
	}

	defer func() {
		perr := recover()
		if perr != nil || rerr != nil {
			if rerr == nil {
				rerr = common.HandlePanic(perr)
			}
			request.Status = common.FAILED
			processor.writeRequest(ctx, &request)
		}
	}()

	log.Printf("Got request: %v", request)

	request.Status = common.IN_PROGRESS
	err = processor.writeRequest(ctx, &request)
	if err != nil {
		return err
	}

	log.Printf("Set request %v status to IN_PROGRESS", request.ID)

	for {
		containerInfo, err := processor.findRunningContainer(ctx, request.ID)
		if err != nil {
			return err
		}

		if containerInfo.ExposedPort != "" {
			processor.fillRequestWithContainerInfo(&request, &containerInfo)
			break
		}

		if processor.creatorMutex.TryLock() {
			defer processor.creatorMutex.Unlock()
			containerInfo, err = processor.createNewContainer(ctx, request.ID)
			if err != nil {
				return err
			}

			if containerInfo.ExposedPort == "" {
				return errors.New("StartNewContainer didn't return port")
			}

			processor.fillRequestWithContainerInfo(&request, &containerInfo)
			break
		}

		time.Sleep(time.Duration(processor.LookupCooldownMillisecond) * time.Millisecond)
	}

	log.Printf("Finished request: %v", request.ID)

	request.Status = common.DONE
	err = processor.writeRequest(ctx, &request)
	if err != nil {
		return err
	}

	log.Printf("Set request %v status to DONE", request.ID)

	return nil
}

func (processor *Processor) findRunningContainer(ctx context.Context, requestID string) (interactor.ContainerInfo, error) {
	log.Printf("Looking for available containers")

	containers, err := processor.DockerClient.ListContainers()
	if err != nil {
		return interactor.ContainerInfo{}, err
	}

	for _, containerID := range containers {
		containerInfo, err := processor.DockerClient.InspectContainer(containerID)
		if err != nil {
			log.Printf("Failed InspectContainer on container %v: %v", containerID, err)
			continue
		}

		reserved, err := processor.reserveContainer(containerInfo.Address, requestID, false)
		if err != nil {
			log.Printf("Failed reserve request on container %v: %v", containerID, err)
			continue
		}

		if reserved {
			log.Printf("Found available container %v", containerID)

			return containerInfo, nil
		}
	}

	log.Printf("No available containers found")

	return interactor.ContainerInfo{}, nil
}

func (processor *Processor) createNewContainer(ctx context.Context, requestID string) (interactor.ContainerInfo, error) {
	id, err := processor.DockerClient.CreateContainer()
	if err != nil {
		return interactor.ContainerInfo{}, err
	}

	containerInfo, err := processor.DockerClient.InspectContainer(id)
	if err != nil {
		return interactor.ContainerInfo{}, err
	}

	reserved, err := processor.reserveContainer(containerInfo.Address, requestID, true)
	if err != nil {
		return interactor.ContainerInfo{}, err
	}

	if !reserved {
		return interactor.ContainerInfo{}, errors.New("container failed to reserve a slot")
	}

	return containerInfo, nil
}

func (processor *Processor) reserveContainer(hostname string, requestID string, retry bool) (bool, error) {
	containerURL := "http://" + hostname + ":" + processor.ImageControlPort
	containerURL += "/reservation/" + requestID

	var err error
	retriesCounter := 0
	for {
		req, err := http.NewRequest("POST", containerURL, nil)
		if err != nil {
			return false, err
		}

		resp, err := processor.HttpClient.Do(req)
		if err == nil {
			return resp.StatusCode == 200, nil
		}

		retriesCounter++
		if !retry || retriesCounter >= processor.ReservationRetries {
			break
		}

		time.Sleep(time.Duration(processor.ReservationCooldown) * time.Millisecond)
	}

	return false, err
}
