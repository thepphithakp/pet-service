package model

import "github.com/vertex/pet-service/internal/domain"

type Permission struct {
	ID          string `gorm:"primaryKey"`
	Name        string
	Description string
	IsActive    bool `gorm:"default:true"`
}

func (Permission) TableName() string { return "pet_permissions" }

func (m *Permission) ToDomain() domain.PetPermission {
	return domain.PetPermission{ID: m.ID, Name: m.Name, Description: m.Description, IsActive: m.IsActive}
}

func PermissionFromDomain(p domain.PetPermission) Permission {
	return Permission{ID: p.ID, Name: p.Name, Description: p.Description, IsActive: p.IsActive}
}
