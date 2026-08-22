package domain

import (
	"time"

	"github.com/google/uuid"
)

// PetCaregiver represents a user who co‑manages a pet.
type PetCaregiver struct {
	ID          uuid.UUID       `json:"id"`
	PetID       uuid.UUID       `json:"petId"`
	UserID      uuid.UUID       `json:"userId"`
	Permissions []PetPermission `json:"permissions,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	CreatedBy   *string         `json:"createdBy,omitempty"`
	UpdatedBy   *string         `json:"updatedBy,omitempty"`
}
