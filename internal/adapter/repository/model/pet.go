// Package model รวม GORM model ที่แยกจาก domain entity
// domain ไม่มี tag ของ framework ใดๆ ส่วน model รู้เรื่องตารางและ column
package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vertex/pet-service/internal/domain"
)

type Pet struct {
	ID               uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OwnerID          uuid.UUID `gorm:"type:uuid;index;not null"`
	OwnerUsername    string    `gorm:"type:varchar(100);index"`
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
	DeletedAt        gorm.DeletedAt `gorm:"index"`
	CreatedBy        *string
	UpdatedBy        *string
	Caregivers       []Caregiver `gorm:"foreignKey:PetID;constraint:OnDelete:CASCADE;"`
}

func (Pet) TableName() string { return "pets" }

func (m *Pet) ToDomain() domain.Pet {
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

func PetFromDomain(p domain.Pet) Pet {
	return Pet{
		ID: p.ID, OwnerID: p.OwnerID, OwnerUsername: p.OwnerUsername, Name: p.Name, Species: p.Species,
		Breed: p.Breed, ColorCode: p.ColorCode, BirthDate: p.BirthDate,
		Gender: p.Gender, AvatarData: p.AvatarData, CurrentWeight: p.CurrentWeight,
		MicrochipId: p.MicrochipId, IsSpayedNeutered: p.IsSpayedNeutered,
		BloodType: p.BloodType, Allergies: p.Allergies, Personality: p.Personality,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, CreatedBy: p.CreatedBy, UpdatedBy: p.UpdatedBy,
	}
}
