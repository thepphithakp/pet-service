package repository

import (
	"time"

	"github.com/google/uuid"
	"github.com/vertex/pet-service/internal/domain"
	"gorm.io/gorm"
)

// --- GORM Models (separate from domain entities) ---

type PetModel struct {
	ID               uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OwnerID          uuid.UUID      `gorm:"type:uuid;index;not null"`
	OwnerUsername    string         `gorm:"type:varchar(100);index"`
	Name             string
	Species          string
	Breed            string
	ColorCode        string
	BirthDate        time.Time
	Gender           string
	AvatarData       []byte
	CurrentWeight    *float64
	MicrochipId      *string
	IsSpayedNeutered bool
	BloodType        *string
	Allergies        *string
	Personality      *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt  `gorm:"index"`
	CreatedBy        *string
	UpdatedBy        *string
	Caregivers       []CaregiverModel `gorm:"foreignKey:PetID;constraint:OnDelete:CASCADE;"`
}

func (PetModel) TableName() string { return "pets" }

func (m *PetModel) ToDomain() domain.Pet {
	pet := domain.Pet{
		ID: m.ID, OwnerID: m.OwnerID, OwnerUsername: m.OwnerUsername, Name: m.Name, Species: m.Species,
		Breed: m.Breed, ColorCode: m.ColorCode, BirthDate: m.BirthDate,
		Gender: m.Gender, AvatarData: m.AvatarData, CurrentWeight: m.CurrentWeight,
		MicrochipId: m.MicrochipId, IsSpayedNeutered: m.IsSpayedNeutered,
		BloodType: m.BloodType, Allergies: m.Allergies, Personality: m.Personality,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, CreatedBy: m.CreatedBy, UpdatedBy: m.UpdatedBy,
	}
	for _, c := range m.Caregivers {
		pet.Caregivers = append(pet.Caregivers, c.ToDomain())
	}
	return pet
}

func PetModelFromDomain(p domain.Pet) PetModel {
	return PetModel{
		ID: p.ID, OwnerID: p.OwnerID, OwnerUsername: p.OwnerUsername, Name: p.Name, Species: p.Species,
		Breed: p.Breed, ColorCode: p.ColorCode, BirthDate: p.BirthDate,
		Gender: p.Gender, AvatarData: p.AvatarData, CurrentWeight: p.CurrentWeight,
		MicrochipId: p.MicrochipId, IsSpayedNeutered: p.IsSpayedNeutered,
		BloodType: p.BloodType, Allergies: p.Allergies, Personality: p.Personality,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, CreatedBy: p.CreatedBy, UpdatedBy: p.UpdatedBy,
	}
}

// ---

type PermissionModel struct {
	ID          string `gorm:"primaryKey"`
	Name        string
	Description string
	IsActive    bool `gorm:"default:true"`
}

func (PermissionModel) TableName() string { return "pet_permissions" }

func (m *PermissionModel) ToDomain() domain.PetPermission {
	return domain.PetPermission{ID: m.ID, Name: m.Name, Description: m.Description, IsActive: m.IsActive}
}

func PermissionModelFromDomain(p domain.PetPermission) PermissionModel {
	return PermissionModel{ID: p.ID, Name: p.Name, Description: p.Description, IsActive: p.IsActive}
}

// ---

type CaregiverModel struct {
	ID          uuid.UUID         `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	PetID       uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:idx_pet_user"`
	UserID      uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:idx_pet_user"`
	Permissions []PermissionModel `gorm:"many2many:caregiver_permissions;constraint:OnDelete:CASCADE;"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
	CreatedBy   *string
	UpdatedBy   *string
}

func (CaregiverModel) TableName() string { return "pet_caregivers" }

func (m *CaregiverModel) ToDomain() domain.PetCaregiver {
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

// ---

type LitterModel struct {
    ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
    PetID     uuid.UUID `gorm:"type:uuid;index:idx_litter_pet_date;not null"`
    Date      time.Time `gorm:"index:idx_litter_pet_date,sort:desc"`
    Type      string
    Amount    int
    CreatedAt time.Time
    UpdatedAt time.Time
    CreatedBy *string
    IsActive  bool `gorm:"default:true"`
}

func (LitterModel) TableName() string { return "litter_logs" }

func (m *LitterModel) ToDomain() domain.LitterLog {
	return domain.LitterLog{
		ID: m.ID, PetID: m.PetID, Date: m.Date, Type: m.Type, Amount: m.Amount,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, CreatedBy: m.CreatedBy, IsActive: m.IsActive,
	}
}

func LitterModelFromDomain(l domain.LitterLog) LitterModel {
	return LitterModel{
		ID: l.ID, PetID: l.PetID, Date: l.Date, Type: l.Type, Amount: l.Amount,
		CreatedAt: l.CreatedAt, CreatedBy: l.CreatedBy, IsActive: l.IsActive,
	}
}

// ---

type WaterModel struct {
    ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
    PetID     uuid.UUID `gorm:"type:uuid;index:idx_water_pet_date;not null"`
    Date      time.Time `gorm:"index:idx_water_pet_date,sort:desc"`
    Amount    int
    CreatedAt time.Time
    UpdatedAt time.Time
    CreatedBy *string
    IsActive  bool `gorm:"default:true"`
}

func (WaterModel) TableName() string { return "water_logs" }

func (m *WaterModel) ToDomain() domain.WaterLog {
	return domain.WaterLog{
		ID: m.ID, PetID: m.PetID, Date: m.Date, Amount: m.Amount,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, CreatedBy: m.CreatedBy, IsActive: m.IsActive,
	}
}

func WaterModelFromDomain(l domain.WaterLog) WaterModel {
	return WaterModel{
		ID: l.ID, PetID: l.PetID, Date: l.Date, Amount: l.Amount,
		CreatedAt: l.CreatedAt, CreatedBy: l.CreatedBy, IsActive: l.IsActive,
	}
}
