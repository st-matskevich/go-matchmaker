package common

import "errors"

const (
	CREATED     = "CREATED"
	IN_PROGRESS = "IN_PROGRESS"
	DONE        = "DONE"
	FAILED      = "FAILED"
	OCCUPIED    = "OCCUPIED"
)

type RequestBody struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Server    string `json:"server,omitempty"`
	Container string `json:"container,omitempty"`
}

const REDIS_DB_ID = 0
const REDIS_QUEUE_LIST_KEY = "queue"

func HandlePanic(perr interface{}) error {
	switch x := perr.(type) {
	case string:
		return errors.New(x)
	case error:
		return x
	default:
		return errors.New("unknown panic")
	}
}
