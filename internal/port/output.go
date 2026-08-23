package port

import (
	"context"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
)

// PetRepository is the driven port (output port) for pet persistence.
// Application services depend on this interface, NOT on GORM directly.
type PetRepository interface {
	// FindAccess ตอบว่า actor คนนี้เกี่ยวข้องกับสัตว์เลี้ยงตัวนี้อย่างไร ด้วย query เดียว
	//
	// คืน AccessNone ทั้งกรณี "ไม่มีสิทธิ์" และ "ไม่มีสัตว์เลี้ยงตัวนี้อยู่จริง"
	// โดยตั้งใจ — ผู้เรียกจะได้ตอบ 404 เหมือนกันทั้งสองกรณี ไม่ให้ไล่เดา UUID ได้
	FindAccess(ctx context.Context, petID, userID uuid.UUID) (domain.PetAccess, error)

	FindAllForUser(ctx context.Context, userID uuid.UUID) ([]domain.Pet, error)
	FindAll(ctx context.Context) ([]domain.Pet, error)

	// FindAllForUserSummary เหมือน FindAllForUser แต่ไม่ดึง avatar_data
	//
	// ต้องแยกเป็นอีก method ไม่ใช่ใส่ flag เพราะชนิดที่คืนต่างกัน
	// และเพื่อให้เห็นชัดในโค้ดว่า query ไหนลากรูปมาด้วย
	FindAllForUserSummary(ctx context.Context, userID uuid.UUID) ([]domain.PetSummary, error)
	FindAllSummary(ctx context.Context) ([]domain.PetSummary, error)

	// FindAvatar ดึงเฉพาะรูป — ไม่ดึงคอลัมน์อื่นเลย
	// คืน nil ทั้งกรณีไม่มีสัตว์เลี้ยงและกรณีมีแต่ไม่มีรูป ผู้เรียกตอบ 404 เหมือนกัน
	FindAvatar(ctx context.Context, petID uuid.UUID) (*domain.Avatar, error)
	FindByID(ctx context.Context, id uuid.UUID) (*domain.Pet, error)
	Save(ctx context.Context, pet *domain.Pet) (*domain.Pet, error)
	Update(ctx context.Context, pet *domain.Pet) (*domain.Pet, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// CaregiverRepository is the driven port for caregiver persistence.
type CaregiverRepository interface {
	FindByPetID(ctx context.Context, petID uuid.UUID) ([]domain.PetCaregiver, error)
	FindByID(ctx context.Context, id uuid.UUID) (*domain.PetCaregiver, error)
	Save(ctx context.Context, caregiver *domain.PetCaregiver) (*domain.PetCaregiver, error)
	// SetPermissions เขียนตาราง join ตรงๆ ด้วย permission ID ที่ validate แล้ว
	// ไม่ใช้ GORM Association.Replace ซึ่ง upsert ตาราง master ให้ด้วย (S-4)
	SetPermissions(ctx context.Context, caregiverID uuid.UUID, permissionIDs []string) (*domain.PetCaregiver, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// LitterRepository is the driven port for litter log persistence.
type LitterRepository interface {
	FindByPetID(ctx context.Context, petID uuid.UUID) ([]domain.LitterLog, error)

	// FindPageByPetID คืนหนึ่งหน้าตามลำดับ (date desc, id desc)
	//
	// ใช้ keyset ไม่ใช่ offset — offset จะข้ามหรือซ้ำรายการเมื่อมี log ใหม่
	// เพิ่มเข้ามาระหว่างที่ผู้ใช้กำลังเลื่อนดู ซึ่งเกิดตลอดกับรายการที่เรียงจากใหม่ไปเก่า
	//
	// คืน hasMore แยกต่างหาก เพราะการนับทั้งตารางเพื่อบอกว่าเหลืออีกไหม
	// แพงกว่าการดึงมาเกินหนึ่งแถวแล้วดูว่าได้ครบไหม
	FindPageByPetID(ctx context.Context, petID uuid.UUID, page domain.LogPage) (logs []domain.LitterLog, hasMore bool, err error)
	Save(ctx context.Context, log *domain.LitterLog) (*domain.LitterLog, error)
	SaveBatch(ctx context.Context, logs []domain.LitterLog) ([]domain.LitterLog, error)
	// Delete รับ petID ด้วยเพื่อยืนยันว่า log อยู่ใต้สัตว์เลี้ยงตัวนั้นจริง
	// เดิมรับแค่ logID ทำให้ลบ log ของสัตว์เลี้ยงตัวอื่นผ่าน URL ของตัวเองได้
	Delete(ctx context.Context, petID, logID uuid.UUID) error
}

// MasterDataRepository จัดการ master data ที่ backoffice แก้ได้
type MasterDataRepository interface {
	FindAll(ctx context.Context, t domain.MasterDataType, includeInactive bool) ([]domain.MasterDataItem, error)
	FindByCode(ctx context.Context, t domain.MasterDataType, code string) (*domain.MasterDataItem, error)
	Create(ctx context.Context, t domain.MasterDataType, item domain.MasterDataItem) (*domain.MasterDataItem, error)
	// Update ใช้ optimistic locking — ถ้า version ไม่ตรงคืน ErrVersionConflict
	Update(ctx context.Context, t domain.MasterDataType, item domain.MasterDataItem) (*domain.MasterDataItem, error)
	// CountUsage นับว่ามีข้อมูลจริงอ้างถึงรายการนี้กี่แถว
	// ใช้เตือนก่อนปิดการใช้งาน
	CountUsage(ctx context.Context, t domain.MasterDataType, code string) (int64, error)
}

// CapabilityRepository อ่าน mapping role → capability ของ pet-service
type CapabilityRepository interface {
	// HasAny คืน true ถ้า role ใดใน roles มี capability ใดใน capabilities
	HasAny(ctx context.Context, roles []string, capabilities ...string) (bool, error)
}

// PermissionRepository อ่าน master data ของสิทธิ์ caregiver
//
// ไม่มี Seed แล้ว — การ seed ย้ายไป db/codeowned/R__0010_pet_permissions.sql
// ซึ่ง Flyway จัดการให้ (ตารางนี้เป็นชั้น A code-owned ไม่เปิดให้ backoffice แก้)
type PermissionRepository interface {
	FindAll(ctx context.Context) ([]domain.PetPermission, error)
}
