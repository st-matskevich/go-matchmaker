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
	ID         string `json:"id"`
	Status     string `json:"status"`
	ServerPort string `json:"port,omitempty"`
	Container  string `json:"container,omitempty"`
}

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
