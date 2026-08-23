package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
)

type fakeCaregiverRepo struct {
	byID    *domain.PetCaregiver
	findErr error

	saved      *domain.PetCaregiver
	setPermsTo []string
	deletedID  uuid.UUID
}

func (f *fakeCaregiverRepo) FindByPetID(context.Context, uuid.UUID) ([]domain.PetCaregiver, error) {
	return nil, f.findErr
}
func (f *fakeCaregiverRepo) FindByID(context.Context, uuid.UUID) (*domain.PetCaregiver, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	return f.byID, nil
}
func (f *fakeCaregiverRepo) Save(_ context.Context, c *domain.PetCaregiver) (*domain.PetCaregiver, error) {
	f.saved = c
	return c, nil
}
func (f *fakeCaregiverRepo) SetPermissions(_ context.Context, _ uuid.UUID, ids []string) (*domain.PetCaregiver, error) {
	f.setPermsTo = ids
	return f.byID, nil
}
func (f *fakeCaregiverRepo) Delete(_ context.Context, id uuid.UUID) error {
	f.deletedID = id
	return nil
}

func caregiverSvc(t *testing.T, repo *fakeCaregiverRepo, perms *fakePermRepo, level domain.AccessLevel) *CaregiverService {
	t.Helper()
	petRepo := &fakePetRepo{access: domain.PetAccess{Level: level}}
	return NewCaregiverService(repo, perms, NewAuthorizer(petRepo, adminCaps()))
}

// activePerms สร้าง master data ของ permission ที่เปิดใช้งานอยู่
func activePerms(ids ...string) *fakePermRepo {
	items := make([]domain.PetPermission, 0, len(ids))
	for _, id := range ids {
		items = append(items, domain.PetPermission{ID: id, IsActive: true})
	}
	return &fakePermRepo{items: items}
}

// TestUpdatePermissions_RejectsUnknownPermission
//
// S-4: เดิมรับ object เต็มก้อนจาก request แล้วส่งให้ GORM ซึ่ง upsert
// ตาราง master ให้ด้วย — client จึงสร้าง permission ID ใหม่ตามใจได้
// ตอนนี้ต้องปฏิเสธ ID ที่ไม่มีใน master data
func TestUpdatePermissions_RejectsUnknownPermission(t *testing.T) {
	petID := uuid.New()
	repo := &fakeCaregiverRepo{byID: &domain.PetCaregiver{ID: uuid.New(), PetID: petID}}
	svc := caregiverSvc(t, repo, activePerms("MANAGE_WATER"), domain.AccessOwner)

	_, err := svc.UpdatePermissions(ctxAs(uuid.New()), repo.byID.ID,
		[]string{"MANAGE_WATER", "สิทธิ์ที่แต่งขึ้นเอง"})

	if !errors.Is(err, domain.ErrInvalidPermission) {
		t.Fatalf("ต้องเป็น ErrInvalidPermission ได้ %v", err)
	}
	if repo.setPermsTo != nil {
		t.Error("ห้ามเขียนอะไรลงฐานข้อมูลเมื่อมี permission ที่ไม่รู้จัก")
	}
}

// TestUpdatePermissions_RejectsInactivePermission
//
// permission ที่ถูกปิดใช้งานใน master data ต้องใช้ไม่ได้อีก
func TestUpdatePermissions_RejectsInactivePermission(t *testing.T) {
	petID := uuid.New()
	repo := &fakeCaregiverRepo{byID: &domain.PetCaregiver{ID: uuid.New(), PetID: petID}}
	perms := &fakePermRepo{items: []domain.PetPermission{
		{ID: "MANAGE_WATER", IsActive: true},
		{ID: "MANAGE_LEGACY", IsActive: false},
	}}
	svc := caregiverSvc(t, repo, perms, domain.AccessOwner)

	if _, err := svc.UpdatePermissions(ctxAs(uuid.New()), repo.byID.ID,
		[]string{"MANAGE_LEGACY"}); !errors.Is(err, domain.ErrInvalidPermission) {
		t.Fatalf("permission ที่ปิดใช้งานต้องถูกปฏิเสธ ได้ %v", err)
	}
}

// TestUpdatePermissions_DeduplicatesAndKeepsOrder
func TestUpdatePermissions_DeduplicatesAndKeepsOrder(t *testing.T) {
	petID := uuid.New()
	repo := &fakeCaregiverRepo{byID: &domain.PetCaregiver{ID: uuid.New(), PetID: petID}}
	svc := caregiverSvc(t, repo, activePerms("MANAGE_WATER", "MANAGE_LITTER"), domain.AccessOwner)

	if _, err := svc.UpdatePermissions(ctxAs(uuid.New()), repo.byID.ID,
		[]string{"MANAGE_WATER", "MANAGE_LITTER", "MANAGE_WATER"}); err != nil {
		t.Fatalf("ไม่ควร error: %v", err)
	}

	want := []string{"MANAGE_WATER", "MANAGE_LITTER"}
	if len(repo.setPermsTo) != len(want) {
		t.Fatalf("ได้ %v ต้องได้ %v — ค่าซ้ำต้องถูกตัดออก", repo.setPermsTo, want)
	}
	for i := range want {
		if repo.setPermsTo[i] != want[i] {
			t.Errorf("ลำดับเปลี่ยน: ได้ %v ต้องได้ %v", repo.setPermsTo, want)
		}
	}
}

// TestCaregiverOperations_RequireOwnerAccess
//
// จัดการผู้ดูแลเป็นสิทธิ์ของเจ้าของ — ผู้ดูแลด้วยกันทำไม่ได้
func TestCaregiverOperations_RequireOwnerAccess(t *testing.T) {
	petID := uuid.New()
	caregiverID := uuid.New()

	for _, level := range []domain.AccessLevel{domain.AccessCaregiver, domain.AccessNone} {
		t.Run(string(rune('A'+level)), func(t *testing.T) {
			repo := &fakeCaregiverRepo{byID: &domain.PetCaregiver{ID: caregiverID, PetID: petID}}
			svc := caregiverSvc(t, repo, activePerms("MANAGE_WATER"), level)
			ctx := ctxAs(uuid.New())

			if _, err := svc.GetForPet(ctx, petID); err == nil {
				t.Error("GetForPet ต้องถูกปฏิเสธ")
			}
			if _, err := svc.Add(ctx, petID, uuid.New()); err == nil {
				t.Error("Add ต้องถูกปฏิเสธ")
			}
			if _, err := svc.UpdatePermissions(ctx, caregiverID, []string{"MANAGE_WATER"}); err == nil {
				t.Error("UpdatePermissions ต้องถูกปฏิเสธ")
			}
			if err := svc.Remove(ctx, caregiverID); err == nil {
				t.Error("Remove ต้องถูกปฏิเสธ")
			}

			if repo.saved != nil || repo.setPermsTo != nil || repo.deletedID != uuid.Nil {
				t.Error("ห้ามมีการเขียนใดๆ เมื่อไม่มีสิทธิ์")
			}
		})
	}
}

// TestAdd_GeneratesIDAndBindsPet
func TestAdd_GeneratesIDAndBindsPet(t *testing.T) {
	petID := uuid.New()
	userID := uuid.New()
	repo := &fakeCaregiverRepo{}
	svc := caregiverSvc(t, repo, activePerms(), domain.AccessOwner)

	got, err := svc.Add(ctxAs(uuid.New()), petID, userID)
	if err != nil {
		t.Fatalf("ไม่ควร error: %v", err)
	}
	if got.ID == uuid.Nil {
		t.Error("ต้องสร้าง ID ให้")
	}
	if got.PetID != petID || got.UserID != userID {
		t.Errorf("ผูกกับสัตว์เลี้ยง/ผู้ใช้ผิด: %+v", got)
	}
}

// TestRemove_ChecksOwnershipOfTheCaregiverRow
//
// ต้องหา caregiver ก่อนเพื่อรู้ว่าอยู่ใต้สัตว์เลี้ยงตัวไหน แล้วค่อยตรวจสิทธิ์
// ถ้าตรวจจาก petID ที่ผู้เรียกส่งมาเอง จะลบ caregiver ของสัตว์เลี้ยงตัวอื่นได้
func TestRemove_ChecksOwnershipOfTheCaregiverRow(t *testing.T) {
	caregiverID := uuid.New()
	repo := &fakeCaregiverRepo{byID: &domain.PetCaregiver{ID: caregiverID, PetID: uuid.New()}}
	svc := caregiverSvc(t, repo, activePerms(), domain.AccessOwner)

	if err := svc.Remove(ctxAs(uuid.New()), caregiverID); err != nil {
		t.Fatalf("ไม่ควร error: %v", err)
	}
	if repo.deletedID != caregiverID {
		t.Errorf("ลบ id ผิด: %v", repo.deletedID)
	}
}

// TestUpdatePermissions_PropagatesLookupError
func TestUpdatePermissions_PropagatesLookupError(t *testing.T) {
	repo := &fakeCaregiverRepo{findErr: errors.New("อ่านไม่ได้")}
	svc := caregiverSvc(t, repo, activePerms(), domain.AccessOwner)

	if _, err := svc.UpdatePermissions(ctxAs(uuid.New()), uuid.New(), nil); err == nil {
		t.Error("ต้องคืน error เมื่อหา caregiver ไม่ได้")
	}
}
