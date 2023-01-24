package common

const (
	CREATED     = "CREATED"
	IN_PROGRESS = "IN_PROGRESS"
	DONE        = "DONE"
	FAILED      = "FAILED"
)

type RequestBody struct {
	ID     uint64 `json:"id"`
	Status string `json:"status"`
	Server string `json:"server,omitempty"`
}

const REDIS_DB_ID = 0
const REDIS_QUEUE_LIST_KEY = "queue"
