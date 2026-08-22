package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vertex/pet-service/internal/adapter/repository/model"
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
	var models []model.Caregiver
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
	var m model.Caregiver
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

func (r *GORMCaregiverRepository) Save(ctx context.Context, caregiver *domain.PetCaregiver) (*domain.PetCaregiver, error) {
	m := model.Caregiver{
		ID:     caregiver.ID,
		PetID:  caregiver.PetID,
		UserID: caregiver.UserID,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		// C-4: ชน partial unique index idx_pet_user_active → เดิมคืน pg error ดิบเป็น 500
		if isUniqueViolation(err) {
			return nil, domain.ErrCaregiverDuplicate
		}
		return nil, err
	}
	c := m.ToDomain()
	return &c, nil
}

// SetPermissions เขียนตาราง caregiver_permissions ตรงๆ ใน transaction เดียว
//
// ไม่ใช้ Association("Permissions").Replace(...) เพราะ GORM จะ upsert แถวใน
// pet_permissions (ตาราง master) ให้ด้วย ทำให้ client แก้ master data ได้ (S-4)
// การเขียน join table ตรงๆ แตะเฉพาะความสัมพันธ์ ไม่แตะ master เลย
func (r *GORMCaregiverRepository) SetPermissions(ctx context.Context, caregiverID uuid.UUID, permissionIDs []string) (*domain.PetCaregiver, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&model.Caregiver{}, "id = ?", caregiverID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrCaregiverNotFound
			}
			return err
		}
		if err := tx.Exec(
			`DELETE FROM caregiver_permissions WHERE caregiver_model_id = ?`, caregiverID,
		).Error; err != nil {
			return err
		}
		for _, pid := range permissionIDs {
			if err := tx.Exec(
				`INSERT INTO caregiver_permissions (caregiver_model_id, permission_model_id)
				 VALUES (?, ?) ON CONFLICT DO NOTHING`, caregiverID, pid,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.FindByID(ctx, caregiverID)
}

func (r *GORMCaregiverRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Caregiver{}, "id = ?", id).Error
}

// GORMPermissionRepository implements port.PermissionRepository.
type GORMPermissionRepository struct {
	db *gorm.DB
}

func NewGORMPermissionRepository(db *gorm.DB) *GORMPermissionRepository {
	return &GORMPermissionRepository{db: db}
}

func (r *GORMPermissionRepository) FindAll(ctx context.Context) ([]domain.PetPermission, error) {
	var models []model.Permission
	if err := r.db.WithContext(ctx).Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]domain.PetPermission, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}
	return result, nil
}

// isUniqueViolation ตรวจ SQLSTATE 23505 (unique_violation)
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
