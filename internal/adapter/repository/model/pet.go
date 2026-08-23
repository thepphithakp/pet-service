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

// PetSummary คือผลของ query ที่ไม่ดึง avatar_data
//
// ประกาศเป็น struct แยกแทนการใช้ model.Pet กับ Select เพราะถ้าใช้ตัวเดิม
// ฟิลด์ AvatarData จะเป็น nil ซึ่งแยกไม่ออกจาก "ไม่มีรูป" — ต้องมี has_avatar
// ที่คำนวณจากฝั่ง database มาด้วย
type PetSummary struct {
	ID               uuid.UUID
	OwnerID          uuid.UUID
	OwnerUsername    string
	Name             string
	Species          string
	Breed            string
	ColorCode        string
	BirthDate        time.Time
	Gender           string
	CurrentWeight    *float64
	MicrochipId      *string
	IsSpayedNeutered bool
	BloodType        *string
	Allergies        *string
	Personality      *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CreatedBy        *string
	UpdatedBy        *string
	HasAvatar        bool

	// ต้อง preload เหมือน query เดิม
	//
	// ถ้าไม่มี field นี้ การปิดสวิตช์ avatar จะทำให้ caregivers หายไปด้วย
	// ซึ่งเป็นการเปลี่ยนพฤติกรรมที่ไม่ได้ตั้งใจ — สวิตช์นั้นควรเปลี่ยน
	// เรื่องรูปอย่างเดียว
	Caregivers []Caregiver `gorm:"foreignKey:PetID;constraint:OnDelete:CASCADE;"`
}

func (PetSummary) TableName() string { return "pets" }

// summaryColumns คือคอลัมน์ที่ query แบบ summary ดึง
//
// เขียนไว้ที่เดียวเพื่อไม่ให้เผลอใส่ avatar_data กลับเข้าไปตอนแก้ทีหลัง
// has_avatar คำนวณที่ database เพื่อไม่ต้องส่ง bytea ทั้งก้อนข้ามเน็ตเวิร์กมานับ
const summaryColumns = `pets.id, pets.owner_id, pets.owner_username, pets.name,
	pets.species, pets.breed, pets.color_code, pets.birth_date, pets.gender,
	pets.current_weight, pets.microchip_id, pets.is_spayed_neutered,
	pets.blood_type, pets.allergies, pets.personality,
	pets.created_at, pets.updated_at, pets.created_by, pets.updated_by,
	(pets.avatar_data IS NOT NULL AND octet_length(pets.avatar_data) > 0) AS has_avatar`

// SummaryColumns เปิดให้ repository ใช้
func SummaryColumns() string { return summaryColumns }

func (m *PetSummary) ToDomain() domain.PetSummary {
	return domain.PetSummary{
		ID: m.ID, OwnerID: m.OwnerID, OwnerUsername: m.OwnerUsername,
		Name: m.Name, Species: m.Species, Breed: m.Breed, ColorCode: m.ColorCode,
		BirthDate: m.BirthDate, Gender: m.Gender, CurrentWeight: m.CurrentWeight,
		MicrochipId: m.MicrochipId, IsSpayedNeutered: m.IsSpayedNeutered,
		BloodType: m.BloodType, Allergies: m.Allergies, Personality: m.Personality,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
		CreatedBy: m.CreatedBy, UpdatedBy: m.UpdatedBy,
		HasAvatar:  m.HasAvatar,
		Caregivers: caregiversToDomain(m.Caregivers),
	}
}

func caregiversToDomain(cs []Caregiver) []domain.PetCaregiver {
	if len(cs) == 0 {
		return nil
	}
	out := make([]domain.PetCaregiver, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.ToDomain())
	}
	return out
}
