package port

import (
	"context"

	"github.com/google/uuid"
	"github.com/vertex/pet-service/internal/domain"
)

// PetRepository is the driven port (output port) for pet persistence.
// Application services depend on this interface, NOT on GORM directly.
type PetRepository interface {
	FindAllForUser(ctx context.Context, userID uuid.UUID) ([]domain.Pet, error)
	FindAll(ctx context.Context) ([]domain.Pet, error)
	FindByID(ctx context.Context, id uuid.UUID) (*domain.Pet, error)
	Save(ctx context.Context, pet *domain.Pet) (*domain.Pet, error)
	Update(ctx context.Context, pet *domain.Pet) (*domain.Pet, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// CaregiverRepository is the driven port for caregiver persistence.
type CaregiverRepository interface {
	FindByPetID(ctx context.Context, petID uuid.UUID) ([]domain.PetCaregiver, error)
	FindByID(ctx context.Context, id uuid.UUID) (*domain.PetCaregiver, error)
	FindDeletedByPetAndUser(ctx context.Context, petID, userID uuid.UUID) (*domain.PetCaregiver, error)
	Save(ctx context.Context, caregiver *domain.PetCaregiver) (*domain.PetCaregiver, error)
	Restore(ctx context.Context, id uuid.UUID) (*domain.PetCaregiver, error)
	UpdatePermissions(ctx context.Context, caregiverID uuid.UUID, permissions []domain.PetPermission) (*domain.PetCaregiver, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// LitterRepository is the driven port for litter log persistence.
type LitterRepository interface {
	FindByPetID(ctx context.Context, petID uuid.UUID) ([]domain.LitterLog, error)
	Save(ctx context.Context, log *domain.LitterLog) (*domain.LitterLog, error)
	SaveBatch(ctx context.Context, logs []domain.LitterLog) ([]domain.LitterLog, error)
	Delete(ctx context.Context, logID uuid.UUID) error
}

// PermissionRepository is the driven port for permission master data.
type PermissionRepository interface {
	FindAll(ctx context.Context) ([]domain.PetPermission, error)
	Seed(ctx context.Context, permissions []domain.PetPermission) error
}
