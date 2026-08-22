package domain

import (
	"time"

	"github.com/google/uuid"
)

// LitterLog is an event recording a litter box usage.
type LitterLog struct {
	ID                uuid.UUID `json:"id"`
	PetID             uuid.UUID `json:"petId"`
	Date              time.Time `json:"date"`
	Type              string    `json:"type"` // "Poop" or "Pee"
	Amount            int       `json:"amount"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
	CreatedBy         *string   `json:"createdBy,omitempty"`
	CreatedByUsername *string   `json:"createdByUsername,omitempty"`
	IsActive          bool      `json:"isActive"`
}
