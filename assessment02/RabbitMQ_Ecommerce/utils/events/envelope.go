package events

import (
	"encoding/json"
	"time"
)

// Define a estrutura do envelope dos eventos a serem publicados/consumidos
type EventEnvelope struct {
	EventID   string          `json:"event_id"`
	EventType string          `json:"event_type"`
	Producer  string          `json:"producer"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature"`
}
