package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"strconv"
	"time"

	"github.com/go-redis/redis"
	"github.com/st-matskevich/go-matchmaker/common"
	"github.com/streadway/amqp"
)

type Processor struct {
	redisClient *redis.Client
}

func (processor *Processor) ProcessMessage(message amqp.Delivery) error {
	var request common.RequestBody
	err := json.Unmarshal(message.Body, &request)
	if err != nil {
		return err
	}

	log.Printf("Got request: %v", request)

	request.Status = common.IN_PROGRESS
	stringID := strconv.FormatUint(request.ID, 10)
	bytes, err := json.Marshal(request)
	if err != nil {
		return err
	}

	err = processor.redisClient.Set(stringID, string(bytes), 0).Err()
	if err != nil {
		return err
	}

	log.Printf("Set request %v status to IN_PROGRESS", request.ID)

	//simulate some work
	time.Sleep(time.Duration((1000 + rand.Intn(2000)) * int(time.Millisecond)))

	log.Printf("Finished request: %v", request.ID)

	request.Status = common.DONE
	bytes, err = json.Marshal(request)
	if err != nil {
		return err
	}

	err = processor.redisClient.Set(stringID, string(bytes), 0).Err()
	if err != nil {
		return err
	}

	log.Printf("Set request %v status to DONE", request.ID)

	return nil
}
