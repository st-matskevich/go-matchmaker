package processor

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/st-matskevich/go-matchmaker/common"
	"github.com/st-matskevich/go-matchmaker/common/data"
	"github.com/st-matskevich/go-matchmaker/common/web"
	"github.com/st-matskevich/go-matchmaker/maker/processor/interactor"
)

type Processor struct {
	DataProvider data.DataProvider
	DockerClient interactor.ContainerInteractor
	HttpClient   web.HTTPClient

	MaxJobs int

	ImageControlPort string

	LookupCooldown int

	ReservationRetries  int
	ReservationCooldown int

	creatorMutex sync.Mutex
}

func (processor *Processor) fillRequestWithContainerInfo(request *common.RequestBody, info *interactor.ContainerInfo) {
	request.Container = info.Address
	request.ServerPort = info.ExposedPort
}

func (processor *Processor) Process() error {
	log.Printf("Starting processing messages in %v jobs", processor.MaxJobs)

	waitChan := make(chan struct{}, processor.MaxJobs)
	for {
		waitChan <- struct{}{}
		go func() {
			val, err := processor.DataProvider.ListPop()
			if err != nil {
				log.Printf("Redis brpop error: %v", err)
			}

			err = processor.processMessage(val)
			if err != nil {
				log.Printf("Failed to process request (%v): %v", val, err)
			}

			<-waitChan
		}()
	}
}

func (processor *Processor) processMessage(ID string) (rerr error) {
	ctx := context.Background()
	defer func() {
		perr := recover()
		if perr != nil || rerr != nil {
			if rerr == nil {
				rerr = common.HandlePanic(perr)
			}
			locker := common.RequestBody{ID: ID, Status: common.FAILED}
			processor.DataProvider.Set(locker)
		}
	}()

	locker := common.RequestBody{ID: ID, Status: common.IN_PROGRESS}
	request, err := processor.DataProvider.Set(locker)
	if err != nil {
		return err
	}

	if request == nil {
		return errors.New("cannot get request")
	}

	log.Printf("Starting processing request %v", request.ID)

	for {
		containerInfo, err := processor.findRunningContainer(ctx, request.ID)
		if err != nil {
			return err
		}

		if containerInfo.ExposedPort != "" {
			processor.fillRequestWithContainerInfo(request, &containerInfo)
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

			processor.fillRequestWithContainerInfo(request, &containerInfo)
			break
		}

		time.Sleep(time.Duration(processor.LookupCooldown) * time.Millisecond)
	}

	log.Printf("Finished request: %v", request.ID)

	request.Status = common.DONE
	_, err = processor.DataProvider.Set(*request)
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
