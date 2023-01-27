package auth

import (
	"errors"

	"github.com/gofiber/fiber/v2"
)

const CLIENT_ID_CTX_KEY = "client-id"

type Authorizer interface {
	Authorize(header string) (id string, err error)
}

func New(authorizer Authorizer) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		id, err := authorizer.Authorize(authHeader)

		if err == nil {
			c.Locals(CLIENT_ID_CTX_KEY, id)
			return c.Next()
		}

		return c.SendStatus(fiber.StatusUnauthorized)
	}
}

type DummyAuthorizer struct{}

func (authorizer *DummyAuthorizer) Authorize(header string) (id string, err error) {
	if header == "" {
		return "", errors.New("header is empty")
	}

	return header, nil
}
