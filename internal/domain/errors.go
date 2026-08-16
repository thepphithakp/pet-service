package domain

import "errors"

// Sentinel domain errors
var (
	ErrPetNotFound        = errors.New("pet not found")
	ErrCaregiverNotFound  = errors.New("caregiver not found")
	ErrLitterLogNotFound  = errors.New("litter log not found")
	ErrCaregiverDuplicate = errors.New("caregiver already exists for this pet")
	ErrInvalidID          = errors.New("invalid ID format")
)
