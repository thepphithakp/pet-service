package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/vertex/pet-service/internal/adapter/repository/model"
	"github.com/vertex/pet-service/internal/domain"
	"gorm.io/gorm"
)

// GORMWaterRepository implements port.WaterRepository.
type GORMWaterRepository struct {
	db *gorm.DB
}

func NewGORMWaterRepository(db *gorm.DB) *GORMWaterRepository {
	return &GORMWaterRepository{db: db}
}

func (r *GORMWaterRepository) Save(ctx context.Context, log *domain.WaterLog) (*domain.WaterLog, error) {
	m := model.WaterFromDomain(*log)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, err
	}
	created := m.ToDomain()
	return &created, nil
}

func (r *GORMWaterRepository) FindByPetID(ctx context.Context, petID uuid.UUID) ([]domain.WaterLog, error) {
	var models []model.Water
	if err := r.db.WithContext(ctx).Where("pet_id = ?", petID).Order("date desc").Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]domain.WaterLog, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}
	return result, nil
}

func (r *GORMWaterRepository) Delete(ctx context.Context, logID uuid.UUID) error {
	result := r.db.WithContext(ctx).Delete(&model.Water{}, "id = ?", logID)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
