package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/vertex/pet-service/internal/domain"
	"gorm.io/gorm"
)

// GORMCaregiverRepository implements port.CaregiverRepository.
type GORMCaregiverRepository struct {
	db *gorm.DB
}

func NewGORMCaregiverRepository(db *gorm.DB) *GORMCaregiverRepository {
	return &GORMCaregiverRepository{db: db}
}

func (r *GORMCaregiverRepository) FindByPetID(ctx context.Context, petID uuid.UUID) ([]domain.PetCaregiver, error) {
	var models []CaregiverModel
	if err := r.db.WithContext(ctx).Preload("Permissions").Where("pet_id = ?", petID).Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]domain.PetCaregiver, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}
	return result, nil
}

func (r *GORMCaregiverRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.PetCaregiver, error) {
	var m CaregiverModel
	err := r.db.WithContext(ctx).Preload("Permissions").First(&m, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrCaregiverNotFound
		}
		return nil, err
	}
	c := m.ToDomain()
	return &c, nil
}

func (r *GORMCaregiverRepository) FindDeletedByPetAndUser(ctx context.Context, petID, userID uuid.UUID) (*domain.PetCaregiver, error) {
	var m CaregiverModel
	err := r.db.WithContext(ctx).Unscoped().
		Where("pet_id = ? AND user_id = ? AND deleted_at IS NOT NULL", petID, userID).
		First(&m).Error
	if err != nil {
		return nil, err
	}
	c := m.ToDomain()
	return &c, nil
}

func (r *GORMCaregiverRepository) Save(ctx context.Context, caregiver *domain.PetCaregiver) (*domain.PetCaregiver, error) {
	m := CaregiverModel{
		ID:    caregiver.ID,
		PetID: caregiver.PetID,
		UserID: caregiver.UserID,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, err
	}
	c := m.ToDomain()
	return &c, nil
}

func (r *GORMCaregiverRepository) Restore(ctx context.Context, id uuid.UUID) (*domain.PetCaregiver, error) {
	if err := r.db.WithContext(ctx).Unscoped().Model(&CaregiverModel{}).
		Where("id = ?", id).Update("deleted_at", nil).Error; err != nil {
		return nil, err
	}
	return r.FindByID(ctx, id)
}

func (r *GORMCaregiverRepository) UpdatePermissions(ctx context.Context, caregiverID uuid.UUID, permissions []domain.PetPermission) (*domain.PetCaregiver, error) {
	var m CaregiverModel
	if err := r.db.WithContext(ctx).First(&m, "id = ?", caregiverID).Error; err != nil {
		return nil, err
	}
	permModels := make([]PermissionModel, len(permissions))
	for i, p := range permissions {
		permModels[i] = PermissionModelFromDomain(p)
	}
	if err := r.db.WithContext(ctx).Model(&m).Association("Permissions").Replace(permModels); err != nil {
		return nil, err
	}
	return r.FindByID(ctx, caregiverID)
}

func (r *GORMCaregiverRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&CaregiverModel{}, "id = ?", id).Error
}

// GORMPermissionRepository implements port.PermissionRepository.
type GORMPermissionRepository struct {
	db *gorm.DB
}

func NewGORMPermissionRepository(db *gorm.DB) *GORMPermissionRepository {
	return &GORMPermissionRepository{db: db}
}

func (r *GORMPermissionRepository) FindAll(ctx context.Context) ([]domain.PetPermission, error) {
	var models []PermissionModel
	if err := r.db.WithContext(ctx).Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]domain.PetPermission, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}
	return result, nil
}

func (r *GORMPermissionRepository) Seed(ctx context.Context, permissions []domain.PetPermission) error {
	for _, p := range permissions {
		m := PermissionModelFromDomain(p)
		r.db.WithContext(ctx).FirstOrCreate(&m, PermissionModel{ID: p.ID})
	}
	return nil
}
