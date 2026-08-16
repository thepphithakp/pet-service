package domain

import (
	"time"

	"github.com/google/uuid"
)

// Pet is the core domain entity. No framework tags.
type Pet struct {
	ID               uuid.UUID      `json:"id"`
	OwnerID          uuid.UUID      `json:"ownerId"`
	OwnerUsername    string         `json:"ownerUsername"`
	Name             string         `json:"name"`
	Species          string         `json:"species"`
	Breed            string         `json:"breed"`
	ColorCode        string         `json:"colorCode"`
	BirthDate        time.Time      `json:"birthDate"`
	Gender           string         `json:"gender"`
	AvatarData       []byte         `json:"avatarData,omitempty"`
	CurrentWeight    *float64       `json:"currentWeight,omitempty"`
	MicrochipId      *string        `json:"microchipId,omitempty"`
	IsSpayedNeutered bool           `json:"isSpayedNeutered"`
	BloodType        *string        `json:"bloodType,omitempty"`
	Allergies        *string        `json:"allergies,omitempty"`
	Personality      *string        `json:"personality,omitempty"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	CreatedBy        *string        `json:"createdBy,omitempty"`
	UpdatedBy        *string        `json:"updatedBy,omitempty"`
	Caregivers       []PetCaregiver `json:"caregivers,omitempty"`
}
