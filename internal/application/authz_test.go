package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
)

// TestAuthorize_Matrix คือ test สำคัญที่สุดของ Phase 1
//
// ครอบทุกช่องของ (ความสัมพันธ์กับสัตว์เลี้ยง × permission ที่มี × สิ่งที่ต้องการ)
// เพื่อพิสูจน์ว่าช่องโหว่ IDOR ปิดแล้วจริง
func TestAuthorize_Matrix(t *testing.T) {
	petID, userID := uuid.New(), uuid.New()

	cases := []struct {
		name    string
		access  domain.PetAccess
		roles   []string
		req     Requirement
		wantErr error
	}{
		{
			name:    "คนนอก อ่านสัตว์เลี้ยงคนอื่น → 404 ไม่ใช่ 403",
			access:  domain.PetAccess{Level: domain.AccessNone},
			req:     ReqPetRead,
			wantErr: domain.ErrPetNotFound,
		},
		{
			name:    "คนนอก ลบสัตว์เลี้ยงคนอื่น → 404",
			access:  domain.PetAccess{Level: domain.AccessNone},
			req:     ReqPetDelete,
			wantErr: domain.ErrPetNotFound,
		},
		{
			name:   "เจ้าของ อ่านได้",
			access: domain.PetAccess{Level: domain.AccessOwner},
			req:    ReqPetRead,
		},
		{
			name:   "เจ้าของ ลบได้",
			access: domain.PetAccess{Level: domain.AccessOwner},
			req:    ReqPetDelete,
		},
		{
			name:   "เจ้าของ ไม่ต้องมีแถวใน caregiver_permissions ก็แก้โปรไฟล์ได้",
			access: domain.PetAccess{Level: domain.AccessOwner},
			req:    ReqPetUpdate,
		},
		{
			name:   "caregiver อ่านได้",
			access: domain.PetAccess{Level: domain.AccessCaregiver},
			req:    ReqPetRead,
		},
		{
			name:    "caregiver ลบสัตว์เลี้ยงไม่ได้ (ต้องเป็นเจ้าของ) → 403",
			access:  domain.PetAccess{Level: domain.AccessCaregiver},
			req:     ReqPetDelete,
			wantErr: domain.ErrForbidden,
		},
		{
			name:    "caregiver ที่ไม่มี EDIT_PROFILE แก้โปรไฟล์ไม่ได้ → 403",
			access:  domain.PetAccess{Level: domain.AccessCaregiver, Permissions: []string{domain.PermManageLitter}},
			req:     ReqPetUpdate,
			wantErr: domain.ErrForbidden,
		},
		{
			name:   "caregiver ที่มี EDIT_PROFILE แก้โปรไฟล์ได้",
			access: domain.PetAccess{Level: domain.AccessCaregiver, Permissions: []string{domain.PermEditProfile}},
			req:    ReqPetUpdate,
		},
		{
			name:   "caregiver ที่มี MANAGE_LITTER บันทึกการขับถ่ายได้",
			access: domain.PetAccess{Level: domain.AccessCaregiver, Permissions: []string{domain.PermManageLitter}},
			req:    ReqLitterWrite,
		},
		{
			name:    "caregiver ที่มีแค่ MANAGE_LITTER บันทึกน้ำไม่ได้",
			access:  domain.PetAccess{Level: domain.AccessCaregiver, Permissions: []string{domain.PermManageLitter}},
			req:     ReqWaterWrite,
			wantErr: domain.ErrForbidden,
		},
		{
			name:   "caregiver ที่มี MANAGE_WATER บันทึกน้ำได้",
			access: domain.PetAccess{Level: domain.AccessCaregiver, Permissions: []string{domain.PermManageWater}},
			req:    ReqWaterWrite,
		},
		{
			name:    "caregiver จัดการ caregiver คนอื่นไม่ได้",
			access:  domain.PetAccess{Level: domain.AccessCaregiver, Permissions: []string{domain.PermEditProfile}},
			req:     ReqCaregiverManage,
			wantErr: domain.ErrForbidden,
		},
		{
			name:   "SUPER_ADMIN อ่านสัตว์เลี้ยงที่ไม่เกี่ยวข้องได้ (ข้าม ownership)",
			access: domain.PetAccess{Level: domain.AccessNone},
			roles:  []string{domain.RoleSuperAdmin},
			req:    ReqPetRead,
		},
		{
			name:   "SUPER_ADMIN ลบสัตว์เลี้ยงคนอื่นได้",
			access: domain.PetAccess{Level: domain.AccessNone},
			roles:  []string{domain.RoleSuperAdmin},
			req:    ReqPetDelete,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAuthorizer(&fakePetRepo{access: tc.access}, adminCaps())
			err := a.Authorize(ctxAs(userID, tc.roles...), petID, tc.req)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("ต้องผ่าน แต่ได้ %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v ต้องการ %v", err, tc.wantErr)
			}
		})
	}
}

func TestAuthorize_NoActor(t *testing.T) {
	a := NewAuthorizer(&fakePetRepo{}, adminCaps())
	err := a.Authorize(context.Background(), uuid.New(), ReqPetRead)
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("err = %v ต้องการ ErrUnauthenticated", err)
	}
}

func TestAuthorizeGlobal(t *testing.T) {
	a := NewAuthorizer(&fakePetRepo{}, adminCaps())

	if err := a.AuthorizeGlobal(ctxAs(uuid.New(), domain.RoleSuperAdmin), domain.CapPetReadAny); err != nil {
		t.Fatalf("SUPER_ADMIN ต้องผ่าน: %v", err)
	}
	err := a.AuthorizeGlobal(ctxAs(uuid.New(), domain.RoleUser), domain.CapPetReadAny)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("USER ต้องได้ ErrForbidden ได้ %v", err)
	}
}

// TestPetAccess_Has ยืนยันว่าเจ้าของไม่ต้องมีแถว permission
func TestPetAccess_Has(t *testing.T) {
	owner := domain.PetAccess{Level: domain.AccessOwner}
	if !owner.Has(domain.PermManageWater) {
		t.Fatal("เจ้าของต้องมีสิทธิ์ทุกอย่าง")
	}
	none := domain.PetAccess{Level: domain.AccessNone, Permissions: []string{domain.PermManageWater}}
	if none.Has(domain.PermManageWater) {
		t.Fatal("AccessNone ต้องไม่ผ่านแม้จะมี permission ติดมา")
	}
}
