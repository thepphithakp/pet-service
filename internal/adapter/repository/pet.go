package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vertex/pet-service/internal/adapter/repository/model"
	"github.com/vertex/pet-service/internal/domain"
)

// GORMPetRepository implements port.PetRepository.
type GORMPetRepository struct {
	db *gorm.DB
}

func NewGORMPetRepository(db *gorm.DB) *GORMPetRepository {
	return &GORMPetRepository{db: db}
}

// accessRow รับผลจาก query เดียวที่ตอบทั้ง "เป็นเจ้าของไหม" และ "เป็น caregiver ที่มีสิทธิ์อะไรบ้าง"
type accessRow struct {
	IsOwner     bool
	IsCaregiver bool
	Permissions string // string_agg คั่นด้วย comma — ไม่ใช้ array เพื่อเลี่ยงปัญหา driver
}

// FindAccess ตรวจสิทธิ์ด้วย query เดียว ไม่ preload ทั้งก้อน
//
// ใช้ index: idx_pets_owner_active, idx_pet_caregivers_pet_active (V5)
// คืน AccessNone ทั้งกรณีไม่มีสิทธิ์และกรณีไม่มีสัตว์เลี้ยงอยู่จริง
func (r *GORMPetRepository) FindAccess(ctx context.Context, petID, userID uuid.UUID) (domain.PetAccess, error) {
	var row accessRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT
		    (p.owner_id = @userID)  AS is_owner,
		    (c.id IS NOT NULL)      AS is_caregiver,
		    COALESCE(string_agg(cp.permission_model_id, ','), '') AS permissions
		FROM pets p
		LEFT JOIN pet_caregivers c
		       ON c.pet_id = p.id AND c.user_id = @userID AND c.deleted_at IS NULL
		LEFT JOIN caregiver_permissions cp
		       ON cp.caregiver_model_id = c.id
		WHERE p.id = @petID AND p.deleted_at IS NULL
		GROUP BY p.owner_id, c.id`,
		map[string]any{"petID": petID, "userID": userID},
	).Scan(&row).Error
	if err != nil {
		return domain.PetAccess{}, err
	}

	switch {
	case row.IsOwner:
		return domain.PetAccess{Level: domain.AccessOwner}, nil
	case row.IsCaregiver:
		var perms []string
		if row.Permissions != "" {
			perms = strings.Split(row.Permissions, ",")
		}
		return domain.PetAccess{Level: domain.AccessCaregiver, Permissions: perms}, nil
	default:
		// ไม่มีแถว (ไม่มีสัตว์เลี้ยงตัวนี้) หรือมีแต่ไม่เกี่ยวข้อง — ตอบเหมือนกันโดยตั้งใจ
		return domain.PetAccess{Level: domain.AccessNone}, nil
	}
}

func (r *GORMPetRepository) FindAllForUser(ctx context.Context, userID uuid.UUID) ([]domain.Pet, error) {
	var models []model.Pet
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
	var models []model.Pet
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
	var m model.Pet
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
	m := model.PetFromDomain(*pet)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, err
	}
	created := m.ToDomain()
	return &created, nil
}

func (r *GORMPetRepository) Update(ctx context.Context, pet *domain.Pet) (*domain.Pet, error) {
	m := model.PetFromDomain(*pet)
	if err := r.db.WithContext(ctx).Save(&m).Error; err != nil {
		return nil, err
	}
	updated := m.ToDomain()
	return &updated, nil
}

func (r *GORMPetRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Pet{}, "id = ?", id).Error
}

// FindAllForUserSummary ดึงรายการสัตว์เลี้ยงโดยไม่ลาก avatar_data มาด้วย
//
// ก่อนหน้านี้ GET /pets ใช้ SELECT * ซึ่งดึง bytea ของรูปทุกตัวมาด้วย
// บน production ผู้ใช้ที่มีสัตว์เลี้ยง 3 ตัวต้องโหลดเกือบ 4MB (รูปใหญ่สุด 2MB)
// ทั้งที่หน้ารายการแสดงรูปเล็กๆ
func (r *GORMPetRepository) FindAllForUserSummary(ctx context.Context, userID uuid.UUID) ([]domain.PetSummary, error) {
	var models []model.PetSummary
	err := r.db.WithContext(ctx).
		Model(&model.Pet{}).
		Select(model.SummaryColumns()).
		Preload("Caregivers.Permissions").
		Where("owner_id = ?", userID).
		Or("EXISTS (SELECT 1 FROM pet_caregivers WHERE pet_caregivers.pet_id = pets.id AND pet_caregivers.user_id = ? AND pet_caregivers.deleted_at IS NULL)", userID).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	return toSummaries(models), nil
}

func (r *GORMPetRepository) FindAllSummary(ctx context.Context) ([]domain.PetSummary, error) {
	var models []model.PetSummary
	err := r.db.WithContext(ctx).
		Model(&model.Pet{}).
		Select(model.SummaryColumns()).
		Preload("Caregivers.Permissions").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	return toSummaries(models), nil
}

func toSummaries(models []model.PetSummary) []domain.PetSummary {
	out := make([]domain.PetSummary, len(models))
	for i := range models {
		out[i] = models[i].ToDomain()
	}
	return out
}

// FindAvatar ดึงเฉพาะรูปของสัตว์เลี้ยงตัวเดียว
//
// คืน nil ทั้งกรณีไม่มีสัตว์เลี้ยงและกรณีมีแต่ไม่มีรูป
// ผู้เรียกจะได้ตอบ 404 เหมือนกัน ไม่ให้ไล่เดา UUID ว่ามีตัวไหนอยู่บ้าง
func (r *GORMPetRepository) FindAvatar(ctx context.Context, petID uuid.UUID) (*domain.Avatar, error) {
	var row struct {
		AvatarData []byte
	}
	err := r.db.WithContext(ctx).
		Model(&model.Pet{}).
		Select("avatar_data").
		Where("id = ?", petID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(row.AvatarData) == 0 {
		return nil, nil
	}

	// ETag คำนวณจากตัวข้อมูลรูป จึงเปลี่ยนเมื่อรูปเปลี่ยนเท่านั้น
	// ถ้าใช้ updated_at ของ pet จะเปลี่ยนทุกครั้งที่แก้ชื่อหรือน้ำหนัก
	// ทำให้ client ต้องโหลดรูปใหม่ทั้งที่รูปเดิม
	sum := sha256.Sum256(row.AvatarData)
	return &domain.Avatar{
		Data: row.AvatarData,
		ETag: `"` + hex.EncodeToString(sum[:]) + `"`,
	}, nil
}
