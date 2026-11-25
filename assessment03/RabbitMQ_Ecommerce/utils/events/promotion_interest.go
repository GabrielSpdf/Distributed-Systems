package events

import "time"

const (
	PromotionSubscribe   = "SUBSCRIBE"
	PromotionUnsubscribe = "UNSUBSCRIBE"
)

// PromotionInterestPayload representa inscrição ou cancelamento de interesse.
type PromotionInterestPayload struct {
	UserID     string    `json:"user_id"`
	Email      string    `json:"email"`
	Action     string    `json:"action"`
	Categories []string  `json:"categories"`
	OccurredAt time.Time `json:"occurred_at"`
}
