package application

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
)

type fakeMasterRepo struct {
	items  map[domain.MasterDataType][]domain.MasterDataItem
	calls  atomic.Int32
	err    error
	usage  int64
	create *domain.MasterDataItem
	update *domain.MasterDataItem
}

func (f *fakeMasterRepo) FindAll(_ context.Context, t domain.MasterDataType, includeInactive bool) ([]domain.MasterDataItem, error) {
	f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	var out []domain.MasterDataItem
	for _, it := range f.items[t] {
		if includeInactive || it.IsActive {
			out = append(out, it)
		}
	}
	return out, nil
}

func (f *fakeMasterRepo) FindByCode(_ context.Context, t domain.MasterDataType, code string) (*domain.MasterDataItem, error) {
	for _, it := range f.items[t] {
		if it.Code == code {
			cp := it
			return &cp, nil
		}
	}
	return nil, domain.ErrMasterDataNotFound
}

func (f *fakeMasterRepo) Create(_ context.Context, _ domain.MasterDataType, item domain.MasterDataItem) (*domain.MasterDataItem, error) {
	f.create = &item
	return &item, f.err
}

func (f *fakeMasterRepo) Update(_ context.Context, _ domain.MasterDataType, item domain.MasterDataItem) (*domain.MasterDataItem, error) {
	f.update = &item
	return &item, f.err
}

func (f *fakeMasterRepo) CountUsage(context.Context, domain.MasterDataType, string) (int64, error) {
	return f.usage, f.err
}

type fakePermRepo struct{ items []domain.PetPermission }

func (f *fakePermRepo) FindAll(context.Context) ([]domain.PetPermission, error) {
	return f.items, nil
}

func newMasterSvc(repo *fakeMasterRepo) *MasterDataService {
	return NewMasterDataService(repo, &fakePermRepo{}, NewAuthorizer(&fakePetRepo{}, adminCaps()))
}

// TestGetCatBreeds_MatchesLegacyV1Shape คือ golden test ของ API v1
//
// การย้าย master data เข้าฐานข้อมูลต้องไม่ทำให้ค่าที่ client เห็นเปลี่ยน
// ค่าที่คาดหวังคือชุดเดียวกับที่ litter_service.go เคย hardcode ไว้
func TestGetCatBreeds_MatchesLegacyV1Shape(t *testing.T) {
	label := func(s string) *string { return &s }
	repo := &fakeMasterRepo{items: map[domain.MasterDataType][]domain.MasterDataItem{
		domain.MasterCatBreeds: {
			{Code: "SCOTTISH_FOLD", NameEn: "Scottish Fold", LegacyLabel: label("Scottish Fold (หูพับ)"), IsActive: true},
			{Code: "PERSIAN", NameEn: "Persian", LegacyLabel: label("Persian"), IsActive: true},
			{Code: "MIXED", NameEn: "Mixed / Other", LegacyLabel: label("Mixed / Other (พันธุ์ผสม/อื่นๆ)"), IsActive: true},
			{Code: "ปิดอยู่", NameEn: "Inactive", IsActive: false},
		},
	}}

	got := newMasterSvc(repo).GetCatBreeds(context.Background())
	want := []string{"Scottish Fold (หูพับ)", "Persian", "Mixed / Other (พันธุ์ผสม/อื่นๆ)"}

	if len(got) != len(want) {
		t.Fatalf("got %v want %v — รายการที่ปิดอยู่ต้องไม่โผล่", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q want %q", i, got[i], want[i])
		}
	}
}

// รายการที่ admin เพิ่มใหม่ไม่มี legacy_label → ใช้ name_en แทน
func TestDisplayLabel_FallsBackToNameEn(t *testing.T) {
	repo := &fakeMasterRepo{items: map[domain.MasterDataType][]domain.MasterDataItem{
		domain.MasterCatBreeds: {{Code: "NEW_BREED", NameEn: "สายพันธุ์ใหม่", IsActive: true}},
	}}
	got := newMasterSvc(repo).GetCatBreeds(context.Background())
	if len(got) != 1 || got[0] != "สายพันธุ์ใหม่" {
		t.Fatalf("got %v", got)
	}
}

func TestMasterData_Cache(t *testing.T) {
	repo := &fakeMasterRepo{items: map[domain.MasterDataType][]domain.MasterDataItem{
		domain.MasterCatBreeds: {{Code: "A", NameEn: "A", IsActive: true}},
	}}
	svc := newMasterSvc(repo)

	for i := 0; i < 5; i++ {
		svc.GetCatBreeds(context.Background())
	}
	if n := repo.calls.Load(); n != 1 {
		t.Fatalf("ยิง DB %d ครั้ง ต้องการ 1 ครั้ง (ที่เหลือมาจาก cache)", n)
	}

	// แก้ผ่าน admin แล้ว cache ของ replica นี้ต้องถูกล้างทันที
	ctx := ctxAs(uuid.New(), domain.RoleSuperAdmin)
	if _, err := svc.Create(ctx, domain.MasterCatBreeds, domain.MasterDataItem{Code: "B", NameEn: "B"}); err != nil {
		t.Fatal(err)
	}
	svc.GetCatBreeds(context.Background())
	if n := repo.calls.Load(); n != 2 {
		t.Fatalf("หลังแก้ต้องโหลดใหม่ — ยิง DB %d ครั้ง", n)
	}
}

// อ่าน DB ไม่ได้ต้องไม่ทำให้ระบบล่ม — คืน slice ว่างแทนการ panic
func TestMasterData_RepoErrorDoesNotPanic(t *testing.T) {
	repo := &fakeMasterRepo{err: errors.New("db ล่ม")}
	if got := newMasterSvc(repo).GetCatBreeds(context.Background()); got == nil || len(got) != 0 {
		t.Fatalf("got %v ต้องการ slice ว่าง", got)
	}
}

// IsValid ต้องไม่ hardcode enum — ค่าที่ admin เพิ่มใหม่ต้องผ่านทันที
func TestIsValid_AcceptsNewlyAddedCode(t *testing.T) {
	repo := &fakeMasterRepo{items: map[domain.MasterDataType][]domain.MasterDataItem{
		domain.MasterLitterTypes: {
			{Code: "Poop", NameEn: "Poop", IsActive: true},
			{Code: "VOMIT", NameEn: "Vomit", IsActive: true}, // admin เพิ่งเพิ่ม
			{Code: "OLD", NameEn: "Old", IsActive: false},
		},
	}}
	svc := newMasterSvc(repo)
	ctx := context.Background()

	if !svc.IsValid(ctx, domain.MasterLitterTypes, "VOMIT") {
		t.Fatal("ค่าที่ admin เพิ่มใหม่ต้องใช้ได้ทันทีโดยไม่ต้อง deploy")
	}
	if svc.IsValid(ctx, domain.MasterLitterTypes, "OLD") {
		t.Fatal("ค่าที่ปิดอยู่ต้องไม่ผ่าน")
	}
	if svc.IsValid(ctx, domain.MasterLitterTypes, "ไม่มีจริง") {
		t.Fatal("ค่าที่ไม่มีใน master ต้องไม่ผ่าน")
	}
	if svc.IsValid(ctx, domain.MasterLitterTypes, "") {
		t.Fatal("ค่าว่างต้องไม่ผ่าน")
	}
}

func TestMasterDataAdmin_RequiresCapability(t *testing.T) {
	repo := &fakeMasterRepo{items: map[domain.MasterDataType][]domain.MasterDataItem{
		domain.MasterCatBreeds: {{Code: "A", NameEn: "A", IsActive: true, Version: 1}},
	}}
	svc := newMasterSvc(repo)
	user := ctxAs(uuid.New(), domain.RoleUser)
	admin := ctxAs(uuid.New(), domain.RoleSuperAdmin)

	if _, err := svc.ListAll(user, domain.MasterCatBreeds); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("user ธรรมดา ListAll: err = %v", err)
	}
	if _, err := svc.Create(user, domain.MasterCatBreeds, domain.MasterDataItem{Code: "X"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("user ธรรมดา Create: err = %v", err)
	}
	if _, err := svc.Deactivate(user, domain.MasterCatBreeds, "A"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("user ธรรมดา Deactivate: err = %v", err)
	}
	if _, err := svc.ListAll(admin, domain.MasterCatBreeds); err != nil {
		t.Fatalf("admin ต้องผ่าน: %v", err)
	}
}

// Deactivate ต้องเป็น soft deactivate เท่านั้น และบอกจำนวนที่กระทบ
func TestDeactivate_IsSoftAndReportsUsage(t *testing.T) {
	repo := &fakeMasterRepo{
		items: map[domain.MasterDataType][]domain.MasterDataItem{
			domain.MasterCatBreeds: {{Code: "PERSIAN", NameEn: "Persian", IsActive: true, Version: 3}},
		},
		usage: 7,
	}
	svc := newMasterSvc(repo)

	usage, err := svc.Deactivate(ctxAs(uuid.New(), domain.RoleSuperAdmin), domain.MasterCatBreeds, "PERSIAN")
	if err != nil {
		t.Fatal(err)
	}
	if usage != 7 {
		t.Fatalf("usage = %d ต้องการ 7", usage)
	}
	if repo.update == nil || repo.update.IsActive {
		t.Fatal("ต้องเป็นการตั้ง is_active = false ไม่ใช่การลบ")
	}
	if repo.update.Version != 3 {
		t.Fatalf("ต้องส่ง version เดิมไปเพื่อ optimistic locking, ได้ %d", repo.update.Version)
	}
}

func TestMasterData_UnknownType(t *testing.T) {
	svc := newMasterSvc(&fakeMasterRepo{})
	if _, err := svc.List(context.Background(), "ไม่มีจริง"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("err = %v ต้องการ ErrValidation", err)
	}
	if _, err := svc.ListAll(ctxAs(uuid.New(), domain.RoleSuperAdmin), "ไม่มีจริง"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("err = %v ต้องการ ErrValidation", err)
	}
}
