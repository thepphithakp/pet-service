package domain

import (
	"time"

	"github.com/google/uuid"
)

// WaterLog is an event recording a cat's water intake.
type WaterLog struct {
	ID                uuid.UUID `json:"id"`
	PetID             uuid.UUID `json:"petId"`
	Date              time.Time `json:"date"`
	Amount            int       `json:"amount"` // Amount in ml
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
	CreatedBy         *string   `json:"createdBy,omitempty"`
	CreatedByUsername *string   `json:"createdByUsername,omitempty"`
	IsActive          bool      `json:"isActive"`
}
