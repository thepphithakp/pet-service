package port

import (
	"context"
)

type EventLog struct {
	EventType     string                 `json:"eventType"`
	Action        string                 `json:"action"`
	ActorID       string                 `json:"actorId"`
	ActorUsername string                 `json:"actorUsername"`
	EntityID      string                 `json:"entityId"`
	EntityType    string                 `json:"entityType"`
	Payload       map[string]interface{} `json:"payload"`
}

type EventPublisher interface {
	Publish(ctx context.Context, event EventLog)
}
