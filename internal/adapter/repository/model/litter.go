package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
)

type Litter struct {
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

func (Litter) TableName() string { return "litter_logs" }

func (m *Litter) ToDomain() domain.LitterLog {
	return domain.LitterLog{
		ID: m.ID, PetID: m.PetID, Date: m.Date, Type: m.Type, Amount: m.Amount,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, CreatedBy: m.CreatedBy, IsActive: m.IsActive,
	}
}

func LitterFromDomain(l domain.LitterLog) Litter {
	return Litter{
		ID: l.ID, PetID: l.PetID, Date: l.Date, Type: l.Type, Amount: l.Amount,
		CreatedAt: l.CreatedAt, CreatedBy: l.CreatedBy, IsActive: l.IsActive,
	}
}
