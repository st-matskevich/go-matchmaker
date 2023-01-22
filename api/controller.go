package main

import (
	"encoding/json"
	"log"
	"strconv"

	"github.com/go-redis/redis"
	"github.com/gofiber/fiber/v2"
	"github.com/sony/sonyflake"
	"github.com/st-matskevich/go-matchmaker/common"
)

type Controller struct {
	idGenerator *sonyflake.Sonyflake
	redisClient *redis.Client
}

func (controller *Controller) HandleCreateRequest(c *fiber.Ctx) error {
	id, err := controller.idGenerator.NextID()
	if err != nil {
		log.Printf("Sonyflake id generation error: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	body := common.RequestBody{ID: id, Status: common.CREATED}
	bytes, err := json.Marshal(body)
	if err != nil {
		log.Printf("JSON encoder error: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	stringID := strconv.FormatUint(id, 10)
	err = controller.redisClient.Set(stringID, string(bytes), 0).Err()
	if err != nil {
		log.Printf("Redis set error: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	err = controller.redisClient.LPush(common.REDIS_QUEUE_LIST_KEY, string(bytes)).Err()
	if err != nil {
		log.Printf("Redis lpush error: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	return c.Status(fiber.StatusAccepted).SendString(string(bytes))
}

func (controller *Controller) HandleGetRequest(c *fiber.Ctx) error {
	stringID := c.Params("id")

	count, err := controller.redisClient.Exists(stringID).Result()
	if err != nil {
		log.Printf("Redis exists error: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	if count < 1 {
		return c.SendStatus(fiber.StatusNotFound)
	}

	val, err := controller.redisClient.Get(stringID).Result()
	if err != nil {
		log.Printf("Redis get error: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	return c.Status(fiber.StatusOK).SendString(val)
}
