package port

import (
	"context"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
)

// PetUseCase is the driving port (input port) for pet operations.
// HTTP handlers depend on this interface, NOT on the concrete service.
type PetUseCase interface {
	GetAllForUser(ctx context.Context, userID uuid.UUID) ([]domain.Pet, error)
	GetAll(ctx context.Context) ([]domain.Pet, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Pet, error)
	Create(ctx context.Context, pet *domain.Pet, ownerID uuid.UUID) (*domain.Pet, error)
	Update(ctx context.Context, id uuid.UUID, pet *domain.Pet) (*domain.Pet, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// CaregiverUseCase is the driving port for caregiver operations.
type CaregiverUseCase interface {
	GetForPet(ctx context.Context, petID uuid.UUID) ([]domain.PetCaregiver, error)
	Add(ctx context.Context, petID uuid.UUID, userID uuid.UUID) (*domain.PetCaregiver, error)
	UpdatePermissions(ctx context.Context, caregiverID uuid.UUID, permissionIDs []string) (*domain.PetCaregiver, error)
	Remove(ctx context.Context, caregiverID uuid.UUID) error
}

// LitterUseCase is the driving port for litter log operations.
type LitterUseCase interface {
	GetForPet(ctx context.Context, petID uuid.UUID) ([]domain.LitterLog, error)
	Create(ctx context.Context, log *domain.LitterLog) (*domain.LitterLog, error)
	CreateBatch(ctx context.Context, logs []domain.LitterLog) ([]domain.LitterLog, error)
	Delete(ctx context.Context, petID, logID uuid.UUID) error
}

// MasterDataUseCase อ่าน master data สำหรับผู้ใช้ทั่วไป
type MasterDataUseCase interface {
	// GetCatBreeds / GetBloodTypes คืนรูปแบบเดิมของ API v1 (array ของ string)
	// ต้องคืนค่าเหมือนเดิมทุกตัวอักษร — มี golden test เฝ้าอยู่
	GetCatBreeds(ctx context.Context) []string
	GetBloodTypes(ctx context.Context) []string

	// List คืนรูปแบบมีโครงสร้างสำหรับ API v2
	List(ctx context.Context, t domain.MasterDataType) ([]domain.MasterDataItem, error)
	// Permissions คืน master permission ของ caregiver (หน้าตั้งสิทธิ์ที่ backoffice ต้องใช้)
	Permissions(ctx context.Context) ([]domain.PetPermission, error)
	// IsValid ใช้ตรวจว่าค่าที่ client ส่งมามีอยู่ใน master data และยัง active
	IsValid(ctx context.Context, t domain.MasterDataType, code string) bool
}

// MasterDataAdminUseCase จัดการ master data — ต้องการ capability masterdata:write
type MasterDataAdminUseCase interface {
	ListAll(ctx context.Context, t domain.MasterDataType) ([]domain.MasterDataItem, error)
	Create(ctx context.Context, t domain.MasterDataType, item domain.MasterDataItem) (*domain.MasterDataItem, error)
	Update(ctx context.Context, t domain.MasterDataType, item domain.MasterDataItem) (*domain.MasterDataItem, error)
	// Deactivate ปิดการใช้งาน ไม่ลบทิ้ง และคืนจำนวนข้อมูลที่ยังอ้างถึงอยู่
	Deactivate(ctx context.Context, t domain.MasterDataType, code string) (int64, error)
	// UsageCount นับข้อมูลที่อ้างถึง ใช้เตือนก่อนกดปิด
	UsageCount(ctx context.Context, t domain.MasterDataType, code string) (int64, error)
}
