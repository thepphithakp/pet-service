package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
)

type Water struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	PetID     uuid.UUID `gorm:"type:uuid;index:idx_water_pet_date;not null"`
	Date      time.Time `gorm:"index:idx_water_pet_date,sort:desc"`
	Amount    int
	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy *string
	IsActive  bool `gorm:"default:true"`
}

func (Water) TableName() string { return "water_logs" }

func (m *Water) ToDomain() domain.WaterLog {
	return domain.WaterLog{
		ID: m.ID, PetID: m.PetID, Date: m.Date, Amount: m.Amount,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, CreatedBy: m.CreatedBy, IsActive: m.IsActive,
	}
}

func WaterFromDomain(l domain.WaterLog) Water {
	return Water{
		ID: l.ID, PetID: l.PetID, Date: l.Date, Amount: l.Amount,
		CreatedAt: l.CreatedAt, CreatedBy: l.CreatedBy, IsActive: l.IsActive,
	}
}
