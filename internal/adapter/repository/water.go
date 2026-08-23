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

// GORMWaterRepository implements port.WaterRepository.
type GORMWaterRepository struct {
	db *gorm.DB
}

func NewGORMWaterRepository(db *gorm.DB) *GORMWaterRepository {
	return &GORMWaterRepository{db: db}
}

// Save เขียน log ใหม่แบบ idempotent
//
// id มาจาก client ได้ (แอปสร้างเองเพื่อ optimistic update และ offline sync)
// ถ้าเน็ตหลุดแล้วแอปส่งซ้ำด้วย id เดิม การ INSERT ตรงๆ จะชน primary key
// แล้วกลายเป็น 500 ทั้งที่ข้อมูลถูกบันทึกไปแล้วเรียบร้อย
//
// ON CONFLICT DO NOTHING ทำให้ส่งซ้ำแล้วได้แถวเดิมกลับไป ไม่ใช่ error
func (r *GORMWaterRepository) Save(ctx context.Context, log *domain.WaterLog) (*domain.WaterLog, error) {
	m := model.WaterFromDomain(*log)

	tx := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).
		Create(&m)
	if tx.Error != nil {
		return nil, tx.Error
	}

	if tx.RowsAffected == 0 {
		// มีแถวนี้อยู่แล้ว — ถ้าเป็นของสัตว์เลี้ยงตัวเดิม ถือว่าส่งซ้ำ คืนของเดิมไป
		var existing model.Water
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

// ⚠️ ต้องมี tiebreaker เสมอ
//
// date อย่างเดียวไม่พอ — log ที่บันทึกวันเดียวกันจะได้ลำดับไม่แน่นอน
// PostgreSQL ไม่รับประกันลำดับของแถวที่ ORDER BY ตัดสินไม่ได้ และลำดับ
// เปลี่ยนได้จริงหลัง UPDATE หรือ VACUUM ทำให้รายการในแอปสลับที่เองโดยไม่มีสาเหตุ
func (r *GORMWaterRepository) FindByPetID(ctx context.Context, petID uuid.UUID) ([]domain.WaterLog, error) {
	var models []model.Water
	if err := r.db.WithContext(ctx).
		Where("pet_id = ?", petID).
		Order("date desc, id desc").
		Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]domain.WaterLog, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}
	return result, nil
}

// Delete ลบ log โดยยืนยันว่าอยู่ใต้สัตว์เลี้ยงตัวที่ระบุจริง
//
// เดิมไม่เช็ค RowsAffected เลย ทำให้ลบของที่ไม่มีอยู่ก็คืน 204 (C-9)
// ตอนนี้ทำเหมือน litter แล้ว
func (r *GORMWaterRepository) Delete(ctx context.Context, petID, logID uuid.UUID) error {
	result := r.db.WithContext(ctx).Delete(&model.Water{}, "id = ? AND pet_id = ?", logID, petID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrWaterLogNotFound
	}
	return nil
}

// FindPageByPetID คืนหนึ่งหน้าด้วย keyset pagination
//
// ดึงมา limit+1 แถวเพื่อรู้ว่ายังมีต่อไหม โดยไม่ต้อง COUNT ทั้งตาราง
// แถวที่เกินมาไม่ได้ส่งกลับ ใช้เป็นสัญญาณอย่างเดียว
func (r *GORMWaterRepository) FindPageByPetID(ctx context.Context, petID uuid.UUID, page domain.LogPage) ([]domain.WaterLog, bool, error) {
	page = page.Normalize()

	q := r.db.WithContext(ctx).Where("pet_id = ?", petID)
	if page.Cursor != nil {
		// เทียบเป็น row value ทีเดียว ตรงกับลำดับ (date desc, id desc) พอดี
		// เขียนแยกเป็น date < ? OR (date = ? AND id < ?) ก็ได้ผลเท่ากัน
		// แต่แบบ row value อ่านง่ายกว่าและ PostgreSQL ใช้ index ได้เหมือนกัน
		q = q.Where("(date, id) < (?, ?)", page.Cursor.Date, page.Cursor.ID)
	}

	var models []model.Water
	if err := q.Order("date desc, id desc").Limit(page.Limit + 1).Find(&models).Error; err != nil {
		return nil, false, err
	}

	hasMore := len(models) > page.Limit
	if hasMore {
		models = models[:page.Limit]
	}

	result := make([]domain.WaterLog, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}
	return result, hasMore, nil
}
