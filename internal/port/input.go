package port

import (
	"context"

	"github.com/google/uuid"
	"github.com/vertex/pet-service/internal/domain"
)

// PetUseCase is the driving port (input port) for pet operations.
// HTTP handlers depend on this interface, NOT on the concrete service.
type PetUseCase interface {
	GetAllForUser(ctx context.Context, userID uuid.UUID) ([]domain.Pet, error)
	GetAll(ctx context.Context) ([]domain.Pet, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Pet, error)
	Create(ctx context.Context, pet *domain.Pet, ownerID uuid.UUID) (*domain.Pet, error)
	Update(ctx context.Context, id uuid.UUID, pet *domain.Pet) (*domain.Pet, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// CaregiverUseCase is the driving port for caregiver operations.
type CaregiverUseCase interface {
	GetForPet(ctx context.Context, petID uuid.UUID) ([]domain.PetCaregiver, error)
	Add(ctx context.Context, petID uuid.UUID, userID uuid.UUID) (*domain.PetCaregiver, error)
	UpdatePermissions(ctx context.Context, caregiverID uuid.UUID, permissionIDs []string) (*domain.PetCaregiver, error)
	Remove(ctx context.Context, caregiverID uuid.UUID) error
}

// LitterUseCase is the driving port for litter log operations.
type LitterUseCase interface {
	GetForPet(ctx context.Context, petID uuid.UUID) ([]domain.LitterLog, error)
	Create(ctx context.Context, log *domain.LitterLog) (*domain.LitterLog, error)
	CreateBatch(ctx context.Context, logs []domain.LitterLog) ([]domain.LitterLog, error)
	Delete(ctx context.Context, petID, logID uuid.UUID) error
}

// MasterDataUseCase is the driving port for master data queries.
type MasterDataUseCase interface {
	GetCatBreeds(ctx context.Context) []string
	GetBloodTypes(ctx context.Context) []string
}
