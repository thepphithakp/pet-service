package port

import (
	"context"

	"github.com/google/uuid"
	"github.com/vertex/pet-service/internal/domain"
)

type WaterRepository interface {
	Save(ctx context.Context, log *domain.WaterLog) (*domain.WaterLog, error)
	FindByPetID(ctx context.Context, petID uuid.UUID) ([]domain.WaterLog, error)
	Delete(ctx context.Context, logID uuid.UUID) error
}

type WaterUseCase interface {
	Create(ctx context.Context, log *domain.WaterLog) (*domain.WaterLog, error)
	GetByPetID(ctx context.Context, petID uuid.UUID) ([]domain.WaterLog, error)
	Delete(ctx context.Context, logID uuid.UUID) error
}
