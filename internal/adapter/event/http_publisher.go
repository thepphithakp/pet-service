package event

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/vertex/pet-service/internal/port"
)

type HTTPEventPublisher struct {
	EventServiceURL string
}

// NewHTTPEventPublisher รับ URL จากภายนอกแทนการอ่าน env เอง (แก้ A-5)
// การตั้งค่าทั้งหมดรวมอยู่ที่ internal/config แล้ว
func NewHTTPEventPublisher(eventServiceURL string) *HTTPEventPublisher {
	return &HTTPEventPublisher{
		EventServiceURL: eventServiceURL,
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
