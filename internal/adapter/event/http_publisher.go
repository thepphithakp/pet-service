package event

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/vertex/pet-service/internal/port"
)

type HTTPEventPublisher struct {
	EventServiceURL string
}

func NewHTTPEventPublisher() *HTTPEventPublisher {
	url := os.Getenv("EVENT_SERVICE_URL")
	if url == "" {
		url = "http://event-service.vertex.svc.cluster.local:4002"
	}
	return &HTTPEventPublisher{
		EventServiceURL: url,
	}
}

func (p *HTTPEventPublisher) Publish(ctx context.Context, event port.EventLog) {
	// Run in goroutine to not block the main request
	go func(evt port.EventLog) {
		jsonData, err := json.Marshal(evt)
		if err != nil {
			log.Printf("Error marshaling event: %v", err)
			return
		}

		resp, err := http.Post(p.EventServiceURL+"/api/v1/events", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("Error publishing event: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			log.Printf("Failed to publish event, status code: %d", resp.StatusCode)
		}
	}(event)
}
