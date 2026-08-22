package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
)

// CaregiverService implements port.CaregiverUseCase.
type CaregiverService struct {
	repo  port.CaregiverRepository
	perms port.PermissionRepository
	authz *Authorizer
}

func NewCaregiverService(repo port.CaregiverRepository, perms port.PermissionRepository, authz *Authorizer) *CaregiverService {
	return &CaregiverService{repo: repo, perms: perms, authz: authz}
}

func (s *CaregiverService) GetForPet(ctx context.Context, petID uuid.UUID) ([]domain.PetCaregiver, error) {
	if err := s.authz.Authorize(ctx, petID, ReqCaregiverManage); err != nil {
		return nil, err
	}
	return s.repo.FindByPetID(ctx, petID)
}

func (s *CaregiverService) Add(ctx context.Context, petID uuid.UUID, userID uuid.UUID) (*domain.PetCaregiver, error) {
	if err := s.authz.Authorize(ctx, petID, ReqCaregiverManage); err != nil {
		return nil, err
	}

	// V5 เปลี่ยน unique index เป็น partial (WHERE deleted_at IS NULL) แล้ว
	// แถวที่ถูก soft delete จึงไม่กินที่อีกต่อไป — ไม่ต้องมี logic Restore (C-5)
	caregiver := &domain.PetCaregiver{
		ID:     uuid.New(),
		PetID:  petID,
		UserID: userID,
	}
	return s.repo.Save(ctx, caregiver)
}

// UpdatePermissions ตั้งสิทธิ์ของ caregiver จากรายการ permission ID
//
// รับเป็น []string ไม่ใช่ []domain.PetPermission โดยตั้งใจ (S-4)
// เดิมรับ object เต็มก้อนจาก request body แล้วส่งต่อให้ GORM Association.Replace
// ซึ่ง upsert แถวใน pet_permissions (ตาราง master) ให้ด้วย
// → client เปลี่ยนชื่อ/คำอธิบาย หรือสร้าง permission ID ใหม่ตามใจได้
func (s *CaregiverService) UpdatePermissions(ctx context.Context, caregiverID uuid.UUID, permissionIDs []string) (*domain.PetCaregiver, error) {
	caregiver, err := s.repo.FindByID(ctx, caregiverID)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Authorize(ctx, caregiver.PetID, ReqCaregiverManage); err != nil {
		return nil, err
	}

	valid, err := s.validPermissionIDs(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(permissionIDs))
	cleaned := make([]string, 0, len(permissionIDs))
	for _, id := range permissionIDs {
		if !valid[id] {
			return nil, fmt.Errorf("%w: ไม่รู้จัก permission %q", domain.ErrInvalidPermission, id)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		cleaned = append(cleaned, id)
	}

	return s.repo.SetPermissions(ctx, caregiverID, cleaned)
}

func (s *CaregiverService) Remove(ctx context.Context, caregiverID uuid.UUID) error {
	caregiver, err := s.repo.FindByID(ctx, caregiverID)
	if err != nil {
		return err
	}
	if err := s.authz.Authorize(ctx, caregiver.PetID, ReqCaregiverManage); err != nil {
		return err
	}
	return s.repo.Delete(ctx, caregiverID)
}

// validPermissionIDs คืน set ของ permission ที่ active อยู่ใน master data
func (s *CaregiverService) validPermissionIDs(ctx context.Context) (map[string]bool, error) {
	all, err := s.perms.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	valid := make(map[string]bool, len(all))
	for _, p := range all {
		if p.IsActive {
			valid[p.ID] = true
		}
	}
	return valid, nil
}
