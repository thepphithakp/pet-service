package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/vertex/pet-service/internal/adapter/repository/model"
	"github.com/vertex/pet-service/internal/domain"
	"gorm.io/gorm"
)

// GORMLitterRepository implements port.LitterRepository.
type GORMLitterRepository struct {
	db *gorm.DB
}

func NewGORMLitterRepository(db *gorm.DB) *GORMLitterRepository {
	return &GORMLitterRepository{db: db}
}

func (r *GORMLitterRepository) FindByPetID(ctx context.Context, petID uuid.UUID) ([]domain.LitterLog, error) {
	var models []model.Litter
	if err := r.db.WithContext(ctx).Where("pet_id = ?", petID).Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]domain.LitterLog, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}
	return result, nil
}

func (r *GORMLitterRepository) Save(ctx context.Context, log *domain.LitterLog) (*domain.LitterLog, error) {
	m := model.LitterFromDomain(*log)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, err
	}
	created := m.ToDomain()
	return &created, nil
}

func (r *GORMLitterRepository) SaveBatch(ctx context.Context, logs []domain.LitterLog) ([]domain.LitterLog, error) {
	models := make([]model.Litter, len(logs))
	for i, l := range logs {
		models[i] = model.LitterFromDomain(l)
	}
	if err := r.db.WithContext(ctx).Create(&models).Error; err != nil {
		return nil, err
	}
	result := make([]domain.LitterLog, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}
	return result, nil
}

func (r *GORMLitterRepository) Delete(ctx context.Context, logID uuid.UUID) error {
	result := r.db.WithContext(ctx).Delete(&model.Litter{}, "id = ?", logID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("litter log not found")
	}
	return nil
}
