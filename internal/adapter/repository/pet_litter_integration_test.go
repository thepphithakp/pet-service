//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vertex/pet-service/internal/domain"
)

func newPet(owner uuid.UUID) *domain.Pet {
	return &domain.Pet{
		ID:        uuid.New(),
		OwnerID:   owner,
		Name:      "แมวทดสอบ",
		Species:   "CAT",
		Gender:    "Female",
		BirthDate: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestPetRepository_SaveFindDelete(t *testing.T) {
	db := openDB(t)
	repo := NewGORMPetRepository(db)
	ctx := context.Background()

	pet := newPet(uuid.New())
	t.Cleanup(func() { db.Exec("DELETE FROM pets WHERE id = ?", pet.ID) })

	saved, err := repo.Save(ctx, pet)
	if err != nil {
		t.Fatalf("Save ไม่สำเร็จ: %v", err)
	}
	if saved.ID != pet.ID {
		t.Errorf("ต้องใช้ id ที่ส่งไป ได้ %v", saved.ID)
	}

	got, err := repo.FindByID(ctx, pet.ID)
	if err != nil {
		t.Fatalf("FindByID ไม่สำเร็จ: %v", err)
	}
	if got.Name != "แมวทดสอบ" || got.Species != "CAT" {
		t.Errorf("ข้อมูลไม่ตรง: %+v", got)
	}

	if err := repo.Delete(ctx, pet.ID); err != nil {
		t.Fatalf("Delete ไม่สำเร็จ: %v", err)
	}
	if _, err := repo.FindByID(ctx, pet.ID); !errors.Is(err, domain.ErrPetNotFound) {
		t.Errorf("ลบแล้วต้องหาไม่เจอ ได้ %v", err)
	}
}

// TestPetRepository_SaveRejectsUnknownMasterData
//
// species ที่ไม่มีใน master data ต้องถูกปฏิเสธที่ database
// ไม่ใช่เขียนลงไปแล้วค่อยรู้ทีหลังว่าข้อมูลเสีย
func TestPetRepository_SaveRejectsUnknownMasterData(t *testing.T) {
	db := openDB(t)
	repo := NewGORMPetRepository(db)

	pet := newPet(uuid.New())
	pet.Species = "ไม่มีชนิดนี้"
	t.Cleanup(func() { db.Exec("DELETE FROM pets WHERE id = ?", pet.ID) })

	if _, err := repo.Save(context.Background(), pet); err == nil {
		t.Fatal("ต้องถูกปฏิเสธเพราะ species ไม่มีใน master data")
	}
}

func TestPetRepository_FindAll(t *testing.T) {
	db := openDB(t)
	repo := NewGORMPetRepository(db)
	ctx := context.Background()

	pet := newPet(uuid.New())
	t.Cleanup(func() { db.Exec("DELETE FROM pets WHERE id = ?", pet.ID) })
	if _, err := repo.Save(ctx, pet); err != nil {
		t.Fatalf("Save ไม่สำเร็จ: %v", err)
	}

	all, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll ไม่สำเร็จ: %v", err)
	}

	found := false
	for _, p := range all {
		if p.ID == pet.ID {
			found = true
		}
	}
	if !found {
		t.Error("FindAll ต้องเห็นสัตว์เลี้ยงที่เพิ่งสร้าง")
	}
}

// --- litter ---

func makeLitterPet(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	id := makePet(t, db)
	t.Cleanup(func() { db.Exec("DELETE FROM litter_logs WHERE pet_id = ?", id) })
	return id
}

// TestLitterRepository_SaveBatchIsIdempotent
//
// batch เป็นเส้นทาง offline sync ที่แอปส่งซ้ำได้เมื่อเน็ตหลุด
// ถ้าไม่ idempotent การ sync รอบที่สองจะล้มทั้งชุดเพราะรายการเดียวที่ซ้ำ
func TestLitterRepository_SaveBatchIsIdempotent(t *testing.T) {
	db := openDB(t)
	repo := NewGORMLitterRepository(db)
	ctx := context.Background()

	petID := makeLitterPet(t, db)
	logs := []domain.LitterLog{
		{ID: uuid.New(), PetID: petID, Date: time.Now(), Type: "Poop", Amount: 1, IsActive: true},
		{ID: uuid.New(), PetID: petID, Date: time.Now(), Type: "Pee", Amount: 2, IsActive: true},
	}

	if _, err := repo.SaveBatch(ctx, logs); err != nil {
		t.Fatalf("ครั้งแรกไม่สำเร็จ: %v", err)
	}
	if _, err := repo.SaveBatch(ctx, logs); err != nil {
		t.Fatalf("ส่งซ้ำต้องไม่ error: %v", err)
	}

	var n int64
	db.Raw("SELECT count(*) FROM litter_logs WHERE pet_id = ?", petID).Scan(&n)
	if n != 2 {
		t.Errorf("มี %d แถว ต้องมี 2 — ส่งซ้ำต้องไม่เพิ่มแถว", n)
	}
}

// TestLitterRepository_FindPageByPetID
func TestLitterRepository_FindPageByPetID(t *testing.T) {
	db := openDB(t)
	repo := NewGORMLitterRepository(db)
	ctx := context.Background()

	petID := makeLitterPet(t, db)
	base := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	logs := make([]domain.LitterLog, 0, 7)
	for i := 0; i < 7; i++ {
		logs = append(logs, domain.LitterLog{
			ID: uuid.New(), PetID: petID, Date: base.Add(-time.Duration(i) * time.Hour),
			Type: "Poop", Amount: i + 1, IsActive: true,
		})
	}
	if _, err := repo.SaveBatch(ctx, logs); err != nil {
		t.Fatalf("เตรียมข้อมูลไม่สำเร็จ: %v", err)
	}

	page, hasMore, err := repo.FindPageByPetID(ctx, petID, domain.LogPage{Limit: 3})
	if err != nil {
		t.Fatalf("FindPageByPetID ไม่สำเร็จ: %v", err)
	}
	if len(page) != 3 {
		t.Fatalf("ได้ %d รายการ ต้องได้ 3", len(page))
	}
	if !hasMore {
		t.Error("ยังเหลืออีก 4 รายการ hasMore ต้องเป็น true")
	}

	// เดินต่อด้วย cursor จนหมด ต้องได้ครบไม่ซ้ำ
	seen := map[uuid.UUID]bool{}
	for _, l := range page {
		seen[l.ID] = true
	}
	cursor := &domain.LogCursor{Date: page[len(page)-1].Date, ID: page[len(page)-1].ID}

	for range 5 {
		next, more, err := repo.FindPageByPetID(ctx, petID, domain.LogPage{Limit: 3, Cursor: cursor})
		if err != nil {
			t.Fatalf("หน้าถัดไปไม่สำเร็จ: %v", err)
		}
		for _, l := range next {
			if seen[l.ID] {
				t.Errorf("รายการ %s ซ้ำข้ามหน้า", l.ID)
			}
			seen[l.ID] = true
		}
		if !more {
			break
		}
		cursor = &domain.LogCursor{Date: next[len(next)-1].Date, ID: next[len(next)-1].ID}
	}

	if len(seen) != 7 {
		t.Errorf("เดินครบแล้วเจอ %d รายการ ต้องเจอ 7", len(seen))
	}
}

// TestLitterRepository_FindPageEmptyPet
func TestLitterRepository_FindPageEmptyPet(t *testing.T) {
	db := openDB(t)
	repo := NewGORMLitterRepository(db)

	page, hasMore, err := repo.FindPageByPetID(context.Background(), uuid.New(), domain.LogPage{Limit: 10})
	if err != nil {
		t.Fatalf("ไม่ควร error: %v", err)
	}
	if len(page) != 0 || hasMore {
		t.Errorf("สัตว์เลี้ยงที่ไม่มี log ต้องได้ list ว่างและ hasMore=false")
	}
}
