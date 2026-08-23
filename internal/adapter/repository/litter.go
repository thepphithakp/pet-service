package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/vertex/pet-service/internal/adapter/repository/model"
	"github.com/vertex/pet-service/internal/domain"
)

// GORMLitterRepository implements port.LitterRepository.
type GORMLitterRepository struct {
	db *gorm.DB
}

func NewGORMLitterRepository(db *gorm.DB) *GORMLitterRepository {
	return &GORMLitterRepository{db: db}
}

// ⚠️ ต้องมี tiebreaker เสมอ
//
// date อย่างเดียวไม่พอ — log ที่บันทึกวันเดียวกันจะได้ลำดับไม่แน่นอน
// PostgreSQL ไม่รับประกันลำดับของแถวที่ ORDER BY ตัดสินไม่ได้ และลำดับ
// เปลี่ยนได้จริงหลัง UPDATE หรือ VACUUM ทำให้รายการในแอปสลับที่เองโดยไม่มีสาเหตุ
//
// ของเดิมไม่มี ORDER BY เลย (C-11) ทำให้ลำดับของ litter log ไม่แน่นอน
// ต่างจาก water ที่เรียงตาม date อยู่แล้ว
func (r *GORMLitterRepository) FindByPetID(ctx context.Context, petID uuid.UUID) ([]domain.LitterLog, error) {
	var models []model.Litter
	if err := r.db.WithContext(ctx).
		Where("pet_id = ?", petID).
		Order("date desc, id desc").
		Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]domain.LitterLog, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}
	return result, nil
}

// Save เขียน log ใหม่แบบ idempotent — เหตุผลเดียวกับ water repository
func (r *GORMLitterRepository) Save(ctx context.Context, log *domain.LitterLog) (*domain.LitterLog, error) {
	m := model.LitterFromDomain(*log)

	tx := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).
		Create(&m)
	if tx.Error != nil {
		return nil, tx.Error
	}

	if tx.RowsAffected == 0 {
		// มีแถวนี้อยู่แล้ว — ถ้าเป็นของสัตว์เลี้ยงตัวเดิม ถือว่าส่งซ้ำ คืนของเดิมไป
		var existing model.Litter
		err := r.db.WithContext(ctx).
			First(&existing, "id = ? AND pet_id = ?", m.ID, m.PetID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// id ไปชนรายการของสัตว์เลี้ยงตัวอื่น — บอกให้ชัดแทนที่จะเป็น 500
			return nil, domain.ErrLogIDConflict
		}
		if err != nil {
			return nil, err
		}
		found := existing.ToDomain()
		return &found, nil
	}

	created := m.ToDomain()
	return &created, nil
}

func (r *GORMLitterRepository) SaveBatch(ctx context.Context, logs []domain.LitterLog) ([]domain.LitterLog, error) {
	models := make([]model.Litter, len(logs))
	for i, l := range logs {
		models[i] = model.LitterFromDomain(l)
	}
	// batch เป็นเส้นทาง offline sync ที่ส่งซ้ำได้อยู่แล้ว จึงต้อง idempotent
	// เหมือนกัน ไม่งั้น sync รอบที่สองจะล้มทั้งชุดเพราะรายการเดียวที่ซ้ำ
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).
		Create(&models).Error; err != nil {
		return nil, err
	}
	result := make([]domain.LitterLog, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}
	return result, nil
}

// Delete ลบ log โดยยืนยันว่าอยู่ใต้สัตว์เลี้ยงตัวที่ระบุจริง
//
// เดิมไม่เช็ค pet_id ทำให้ยิง DELETE /pets/<ของตัวเอง>/litter-logs/<log ของคนอื่น> ได้
func (r *GORMLitterRepository) Delete(ctx context.Context, petID, logID uuid.UUID) error {
	result := r.db.WithContext(ctx).Delete(&model.Litter{}, "id = ? AND pet_id = ?", logID, petID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// คืน sentinel แทน errors.New ดิบ เพื่อให้ map เป็น 404 ได้ (C-4)
		return domain.ErrLitterLogNotFound
	}
	return nil
}
