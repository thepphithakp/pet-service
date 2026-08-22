package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vertex/pet-service/internal/domain"
)

type Caregiver struct {
	ID          uuid.UUID    `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	PetID       uuid.UUID    `gorm:"type:uuid;not null;uniqueIndex:idx_pet_user"`
	UserID      uuid.UUID    `gorm:"type:uuid;index;not null;uniqueIndex:idx_pet_user"`
	// ⚠️ joinForeignKey / joinReferences ต้องระบุเสมอ ห้ามลบ
	// GORM ตั้งชื่อ column ของ join table จาก "ชื่อ struct" ไม่ใช่ชื่อตาราง
	// struct นี้เคยชื่อ CaregiverModel / PermissionModel ทำให้ column จริงในฐานข้อมูล
	// คือ caregiver_model_id / permission_model_id
	// ถ้าไม่ pin ไว้ การเปลี่ยนชื่อ struct จะทำให้ GORM มองหา caregiver_id / permission_id
	// แล้ว permission ที่ผูกไว้เดิมทั้งหมดจะหายไปเงียบๆ
	// มี TestJoinTableColumnNames เฝ้าอยู่
	Permissions []Permission `gorm:"many2many:caregiver_permissions;joinForeignKey:CaregiverModelID;joinReferences:PermissionModelID;constraint:OnDelete:CASCADE;"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
	CreatedBy   *string
	UpdatedBy   *string
}

func (Caregiver) TableName() string { return "pet_caregivers" }

func (m *Caregiver) ToDomain() domain.PetCaregiver {
	c := domain.PetCaregiver{
		ID: m.ID, PetID: m.PetID, UserID: m.UserID,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
		CreatedBy: m.CreatedBy, UpdatedBy: m.UpdatedBy,
	}
	for _, p := range m.Permissions {
		c.Permissions = append(c.Permissions, p.ToDomain())
	}
	return c
}
