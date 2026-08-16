package application

import (
	"context"

	"github.com/google/uuid"
	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
)

// CaregiverService implements port.CaregiverUseCase.
type CaregiverService struct {
	repo port.CaregiverRepository
}

func NewCaregiverService(repo port.CaregiverRepository) *CaregiverService {
	return &CaregiverService{repo: repo}
}

func (s *CaregiverService) GetForPet(ctx context.Context, petID uuid.UUID) ([]domain.PetCaregiver, error) {
	return s.repo.FindByPetID(ctx, petID)
}

func (s *CaregiverService) Add(ctx context.Context, petID uuid.UUID, userID uuid.UUID) (*domain.PetCaregiver, error) {
	// If previously soft-deleted, restore instead of creating new
	existing, err := s.repo.FindDeletedByPetAndUser(ctx, petID, userID)
	if err == nil && existing != nil {
		return s.repo.Restore(ctx, existing.ID)
	}

	caregiver := &domain.PetCaregiver{
		ID:     uuid.New(),
		PetID:  petID,
		UserID: userID,
	}
	return s.repo.Save(ctx, caregiver)
}

func (s *CaregiverService) UpdatePermissions(ctx context.Context, caregiverID uuid.UUID, permissions []domain.PetPermission) (*domain.PetCaregiver, error) {
	_, err := s.repo.FindByID(ctx, caregiverID)
	if err != nil {
		return nil, err
	}
	return s.repo.UpdatePermissions(ctx, caregiverID, permissions)
}

func (s *CaregiverService) Remove(ctx context.Context, caregiverID uuid.UUID) error {
	_, err := s.repo.FindByID(ctx, caregiverID)
	if err != nil {
		return err
	}
	return s.repo.Delete(ctx, caregiverID)
}
