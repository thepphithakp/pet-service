package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/vertex/pet-service/internal/domain"
	"gorm.io/gorm"
)

// GORMPetRepository implements port.PetRepository.
type GORMPetRepository struct {
	db *gorm.DB
}

func NewGORMPetRepository(db *gorm.DB) *GORMPetRepository {
	return &GORMPetRepository{db: db}
}

func (r *GORMPetRepository) FindAllForUser(ctx context.Context, userID uuid.UUID) ([]domain.Pet, error) {
    var models []PetModel
    err := r.db.WithContext(ctx).
        Preload("Caregivers.Permissions").
        Where("owner_id = ?", userID).
        Or("EXISTS (SELECT 1 FROM pet_caregivers WHERE pet_caregivers.pet_id = pets.id AND pet_caregivers.user_id = ? AND pet_caregivers.deleted_at IS NULL)", userID).
        Find(&models).Error
    if err != nil {
        return nil, err
    }
    pets := make([]domain.Pet, len(models))
    for i, m := range models {
        pets[i] = m.ToDomain()
    }
    return pets, nil
}

func (r *GORMPetRepository) FindAll(ctx context.Context) ([]domain.Pet, error) {
    var models []PetModel
    err := r.db.WithContext(ctx).
        Preload("Caregivers.Permissions").
        Find(&models).Error
    if err != nil {
        return nil, err
    }
    pets := make([]domain.Pet, len(models))
    for i, m := range models {
        pets[i] = m.ToDomain()
    }
    return pets, nil
}



func (r *GORMPetRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Pet, error) {
	var m PetModel
	err := r.db.WithContext(ctx).Preload("Caregivers.Permissions").First(&m, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrPetNotFound
		}
		return nil, err
	}
	pet := m.ToDomain()
	return &pet, nil
}

func (r *GORMPetRepository) Save(ctx context.Context, pet *domain.Pet) (*domain.Pet, error) {
	m := PetModelFromDomain(*pet)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, err
	}
	created := m.ToDomain()
	return &created, nil
}

func (r *GORMPetRepository) Update(ctx context.Context, pet *domain.Pet) (*domain.Pet, error) {
	m := PetModelFromDomain(*pet)
	if err := r.db.WithContext(ctx).Save(&m).Error; err != nil {
		return nil, err
	}
	updated := m.ToDomain()
	return &updated, nil
}

func (r *GORMPetRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&PetModel{}, "id = ?", id).Error
}
