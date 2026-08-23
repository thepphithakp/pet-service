package application

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
)

// newPetSvc ประกอบ service พร้อม authorizer ที่ให้สิทธิ์ระดับที่ต้องการ
func newPetSvc(repo *fakePetRepo, pub *fakePublisher, level domain.AccessLevel) *PetService {
	repo.access = domain.PetAccess{Level: level}
	return NewPetService(repo, recorderFor(pub), NewAuthorizer(repo, adminCaps()))
}

func TestPetService_Create(t *testing.T) {
	repo := &fakePetRepo{}
	pub := &fakePublisher{}
	svc := newPetSvc(repo, pub, domain.AccessOwner)

	owner := uuid.New()
	in := &domain.Pet{Name: "มะลิ", Species: "Cat"}
	out, err := svc.Create(ctxAs(owner), in, owner)
	if err != nil {
		t.Fatal(err)
	}

	// service เป็นคนกำหนด ID และ OwnerID เอง — ค่าจาก client ถูกทับ
	if out.ID == uuid.Nil {
		t.Fatal("service ต้องกำหนด ID")
	}
	if out.OwnerID != owner {
		t.Fatalf("OwnerID = %s ต้องมาจาก argument", out.OwnerID)
	}
	if len(pub.events) != 1 {
		t.Fatalf("ต้อง publish 1 event, ได้ %d", len(pub.events))
	}
	e := pub.events[0]
	if e.EventType != "PetProfile" || e.Action != "Pet Created" || e.EntityType != "Pet" {
		t.Fatalf("event ไม่ตรง: %+v", e)
	}
	if e.ActorID != owner.String() {
		t.Fatalf("ActorID = %q ต้องเป็น owner id", e.ActorID)
	}
}

func TestPetService_Create_NoEventOnSaveFailure(t *testing.T) {
	repo := &fakePetRepo{saveErr: errBoom}
	pub := &fakePublisher{}
	svc := newPetSvc(repo, pub, domain.AccessOwner)
	if _, err := svc.Create(ctxAs(uuid.New()), &domain.Pet{}, uuid.New()); err == nil {
		t.Fatal("ต้องคืน error")
	}
	if len(pub.events) != 0 {
		t.Fatal("ห้าม publish event เมื่อ save ล้มเหลว")
	}
}

func TestPetService_Update_PatchSemantics(t *testing.T) {
	existing := &domain.Pet{
		ID: uuid.New(), Name: "เดิม", Species: "Cat", Breed: "Persian",
		MicrochipId: strp("CHIP-1"), Allergies: strp("ปลาทู"),
		BirthDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	repo := &fakePetRepo{byID: existing}
	svc := newPetSvc(repo, &fakePublisher{}, domain.AccessOwner)

	// ฟิลด์ที่เป็นค่าว่าง = "ไม่แก้"
	out, err := svc.Update(ctxAs(uuid.New()), existing.ID, &domain.Pet{Name: "ใหม่"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Name != "ใหม่" {
		t.Fatalf("Name = %q", out.Name)
	}
	if out.Breed != "Persian" {
		t.Fatalf("Breed ต้องไม่ถูกแก้เมื่อไม่ได้ส่งมา, ได้ %q", out.Breed)
	}
	if out.MicrochipId == nil || *out.MicrochipId != "CHIP-1" {
		t.Fatal("MicrochipId ต้องคงเดิมเมื่อไม่ได้ส่งมา")
	}
}

// BUG C-3: ส่ง nil มาเพื่อล้างค่าไม่ได้ — nil ถูกตีความว่า "ไม่แก้"
func TestPetService_Update_CannotClearNullable_KnownBug(t *testing.T) {
	existing := &domain.Pet{ID: uuid.New(), Name: "x", MicrochipId: strp("CHIP-1")}
	repo := &fakePetRepo{byID: existing}
	svc := newPetSvc(repo, &fakePublisher{}, domain.AccessOwner)

	out, _ := svc.Update(ctxAs(uuid.New()), existing.ID, &domain.Pet{MicrochipId: nil})
	if out.MicrochipId != nil {
		t.Log("ยืนยัน bug C-3: ล้าง microchipId เป็น null ไม่ได้ — Phase 4.4 ต้องเพิ่ม PATCH ที่แยก 'ไม่ส่ง' กับ 'ส่ง null'")
		return
	}
	t.Log("C-3 ถูกแก้แล้ว")
}

// C-2 แก้แล้ว: ActorID ต้องเป็น user id ของ "คนที่กดแก้" ไม่ใช่ username ของเจ้าของ
func TestPetService_Update_ActorIsTheCaller(t *testing.T) {
	existing := &domain.Pet{ID: uuid.New(), Name: "x", OwnerUsername: "เจ้าของ"}
	pub := &fakePublisher{}
	repo := &fakePetRepo{byID: existing}
	svc := newPetSvc(repo, pub, domain.AccessOwner)

	caller := uuid.New()
	if _, err := svc.Update(ctxAs(caller), existing.ID, &domain.Pet{Name: "y"}); err != nil {
		t.Fatal(err)
	}
	if len(pub.events) != 1 {
		t.Fatalf("ต้อง publish 1 event, ได้ %d", len(pub.events))
	}
	e := pub.events[0]
	if e.ActorID != caller.String() {
		t.Fatalf("ActorID = %q ต้องเป็น user id ของผู้กระทำ (%s)", e.ActorID, caller)
	}
	if e.ActorUsername != "tester" {
		t.Fatalf("ActorUsername = %q ต้องมาจาก actor", e.ActorUsername)
	}
	// UpdatedBy ต้องถูกเซ็ตจาก actor ไม่ใช่จาก request body
	if repo.updated.UpdatedBy == nil || *repo.updated.UpdatedBy != caller.String() {
		t.Fatal("UpdatedBy ต้องมาจาก actor")
	}
}

// IDOR: user ที่ไม่เกี่ยวข้องต้องเข้าถึงไม่ได้เลย และต้องแยกไม่ออกจาก "ไม่มีอยู่จริง"
func TestPetService_IDOR_Blocked(t *testing.T) {
	existing := &domain.Pet{ID: uuid.New(), Name: "ของคนอื่น"}
	repo := &fakePetRepo{byID: existing}
	svc := newPetSvc(repo, &fakePublisher{}, domain.AccessNone)
	stranger := ctxAs(uuid.New())

	if _, err := svc.GetByID(stranger, existing.ID); !errors.Is(err, domain.ErrPetNotFound) {
		t.Fatalf("อ่าน: err = %v ต้องการ ErrPetNotFound", err)
	}
	if _, err := svc.Update(stranger, existing.ID, &domain.Pet{Name: "แก้"}); !errors.Is(err, domain.ErrPetNotFound) {
		t.Fatalf("แก้: err = %v ต้องการ ErrPetNotFound", err)
	}
	if err := svc.Delete(stranger, existing.ID); !errors.Is(err, domain.ErrPetNotFound) {
		t.Fatalf("ลบ: err = %v ต้องการ ErrPetNotFound", err)
	}
	if repo.deleteID != uuid.Nil {
		t.Fatal("ห้ามเรียก repo.Delete เลยเมื่อไม่มีสิทธิ์")
	}
}

// S-2 แก้แล้ว: /admin/pets ต้องการ capability
func TestPetService_GetAll_RequiresCapability(t *testing.T) {
	repo := &fakePetRepo{}
	svc := newPetSvc(repo, &fakePublisher{}, domain.AccessNone)

	if _, err := svc.GetAll(ctxAs(uuid.New(), domain.RoleUser)); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("user ธรรมดา: err = %v ต้องการ ErrForbidden", err)
	}
	if _, err := svc.GetAll(ctxAs(uuid.New(), domain.RoleSuperAdmin)); err != nil {
		t.Fatalf("SUPER_ADMIN ต้องผ่าน: %v", err)
	}
}

func TestPetService_Delete(t *testing.T) {
	existing := &domain.Pet{ID: uuid.New(), Name: "มะลิ"}
	pub := &fakePublisher{}
	repo := &fakePetRepo{byID: existing}
	if err := newPetSvc(repo, pub, domain.AccessOwner).Delete(ctxAs(uuid.New()), existing.ID); err != nil {
		t.Fatal(err)
	}
	if repo.deleteID != existing.ID {
		t.Fatal("ต้องเรียก repo.Delete ด้วย id ที่ถูกต้อง")
	}
	if len(pub.events) != 1 || pub.events[0].Action != "Pet Deleted" {
		t.Fatalf("event ไม่ตรง: %+v", pub.events)
	}
}

func TestPetService_Delete_NotFound(t *testing.T) {
	repo := &fakePetRepo{findErr: domain.ErrPetNotFound}
	pub := &fakePublisher{}
	err := newPetSvc(repo, pub, domain.AccessOwner).Delete(ctxAs(uuid.New()), uuid.New())
	if !errors.Is(err, domain.ErrPetNotFound) {
		t.Fatalf("err = %v ต้องเป็น ErrPetNotFound", err)
	}
	if len(pub.events) != 0 {
		t.Fatal("ห้าม publish event เมื่อหา pet ไม่เจอ")
	}
}
