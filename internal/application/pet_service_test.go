package application

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
)

func TestPetService_Create(t *testing.T) {
	repo := &fakePetRepo{}
	pub := &fakePublisher{}
	svc := NewPetService(repo, pub)

	owner := uuid.New()
	in := &domain.Pet{Name: "มะลิ", Species: "Cat"}
	out, err := svc.Create(context.Background(), in, owner)
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
	if _, err := NewPetService(repo, pub).Create(context.Background(), &domain.Pet{}, uuid.New()); err == nil {
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
	svc := NewPetService(repo, &fakePublisher{})

	// ฟิลด์ที่เป็นค่าว่าง = "ไม่แก้"
	out, err := svc.Update(context.Background(), existing.ID, &domain.Pet{Name: "ใหม่"})
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
	svc := NewPetService(repo, &fakePublisher{})

	out, _ := svc.Update(context.Background(), existing.ID, &domain.Pet{MicrochipId: nil})
	if out.MicrochipId != nil {
		t.Log("ยืนยัน bug C-3: ล้าง microchipId เป็น null ไม่ได้ — Phase 4.4 ต้องเพิ่ม PATCH ที่แยก 'ไม่ส่ง' กับ 'ส่ง null'")
		return
	}
	t.Log("C-3 ถูกแก้แล้ว")
}

// BUG C-2: ActorID ถูกยัดด้วย OwnerUsername (username ของเจ้าของ) ไม่ใช่ user id ของคนที่กดแก้
func TestPetService_Update_ActorID_KnownBug(t *testing.T) {
	existing := &domain.Pet{ID: uuid.New(), Name: "x", OwnerUsername: "เจ้าของ"}
	pub := &fakePublisher{}
	svc := NewPetService(&fakePetRepo{byID: existing}, pub)

	if _, err := svc.Update(context.Background(), existing.ID, &domain.Pet{Name: "y"}); err != nil {
		t.Fatal(err)
	}
	if len(pub.events) != 1 {
		t.Fatalf("ต้อง publish 1 event, ได้ %d", len(pub.events))
	}
	if pub.events[0].ActorID == "เจ้าของ" {
		t.Log("ยืนยัน bug C-2: ActorID ถูกใส่ด้วย username ของเจ้าของ ไม่ใช่ id ของผู้กระทำ — Phase 4.3 ต้องส่ง Actor ลงมาถึง service")
		return
	}
	t.Log("C-2 ถูกแก้แล้ว")
}

func TestPetService_Delete(t *testing.T) {
	existing := &domain.Pet{ID: uuid.New(), Name: "มะลิ"}
	pub := &fakePublisher{}
	repo := &fakePetRepo{byID: existing}
	if err := NewPetService(repo, pub).Delete(context.Background(), existing.ID); err != nil {
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
	err := NewPetService(repo, pub).Delete(context.Background(), uuid.New())
	if err != domain.ErrPetNotFound {
		t.Fatalf("err = %v ต้องเป็น ErrPetNotFound", err)
	}
	if len(pub.events) != 0 {
		t.Fatal("ห้าม publish event เมื่อหา pet ไม่เจอ")
	}
}

// ล็อกค่า master data ปัจจุบันไว้เป็น golden — Phase 3 ต้องคืนค่าเดิมทุกตัวอักษร
func TestMasterDataService_GoldenValues(t *testing.T) {
	svc := NewMasterDataService()
	breeds := svc.GetCatBreeds(context.Background())
	if len(breeds) != 14 {
		t.Fatalf("จำนวนสายพันธุ์ = %d ต้องการ 14", len(breeds))
	}
	if breeds[0] != "Scottish Fold (หูพับ)" {
		t.Fatalf("breeds[0] = %q", breeds[0])
	}
	if breeds[13] != "Mixed / Other (พันธุ์ผสม/อื่นๆ)" {
		t.Fatalf("breeds[13] = %q", breeds[13])
	}
	blood := svc.GetBloodTypes(context.Background())
	want := []string{"Unknown", "A", "B", "AB"}
	if len(blood) != len(want) {
		t.Fatalf("blood types = %v", blood)
	}
	for i := range want {
		if blood[i] != want[i] {
			t.Fatalf("blood[%d] = %q want %q", i, blood[i], want[i])
		}
	}
}
