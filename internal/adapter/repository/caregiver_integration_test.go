//go:build integration

package repository

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/vertex/pet-service/internal/domain"
)

// openDB ต่อฐานข้อมูลที่รัน Flyway migration มาแล้ว
//
// กันไม่ให้เผลอรันใส่ production ด้วยการเช็ค seed ของ dev เหมือนที่ bootstrap ทำ
func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ไม่ได้ตั้ง TEST_DATABASE_URL — ข้าม integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("ต่อฐานข้อมูลไม่ได้: %v", err)
	}

	var n int64
	if err := db.Raw(
		`SELECT count(*) FROM flyway_schema_history WHERE description = ? AND success`,
		"9000 dev sample pets").Scan(&n).Error; err != nil || n == 0 {
		t.Fatalf("ฐานข้อมูลนี้ไม่มี seed ของ dev จึงถือว่าไม่ใช่ฐานข้อมูลชั่วคราวที่ทิ้งได้")
	}
	return db
}

// makePet สร้างสัตว์เลี้ยงสำหรับเทสต์แล้วเก็บกวาดให้เอง
func makePet(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	id := uuid.New()
	err := db.Exec(`INSERT INTO pets (id, owner_id, owner_username, name, species, gender,
	                                  is_spayed_neutered, created_at, updated_at)
	                VALUES (?, ?, 'repo-test', 'ทดสอบ', 'CAT', 'Female', false, now(), now())`,
		id, uuid.New()).Error
	if err != nil {
		t.Fatalf("สร้างสัตว์เลี้ยงทดสอบไม่ได้: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM caregiver_permissions WHERE caregiver_model_id IN
		         (SELECT id FROM pet_caregivers WHERE pet_id = ?)`, id)
		db.Exec("DELETE FROM pet_caregivers WHERE pet_id = ?", id)
		db.Exec("DELETE FROM pets WHERE id = ?", id)
	})
	return id
}

func TestCaregiverRepository_SaveAndFind(t *testing.T) {
	db := openDB(t)
	repo := NewGORMCaregiverRepository(db)
	ctx := context.Background()

	petID := makePet(t, db)
	userID := uuid.New()

	saved, err := repo.Save(ctx, &domain.PetCaregiver{
		ID: uuid.New(), PetID: petID, UserID: userID,
	})
	if err != nil {
		t.Fatalf("Save ไม่สำเร็จ: %v", err)
	}

	got, err := repo.FindByID(ctx, saved.ID)
	if err != nil {
		t.Fatalf("FindByID ไม่สำเร็จ: %v", err)
	}
	if got.PetID != petID || got.UserID != userID {
		t.Errorf("ข้อมูลไม่ตรง: %+v", got)
	}
}

// TestCaregiverRepository_DuplicateIsTyped
//
// C-4: ชน partial unique index แล้วต้องได้ error ของโดเมน ไม่ใช่ pg error ดิบ
// ซึ่งจะกลายเป็น 500 ทั้งที่เป็นความผิดของ request
func TestCaregiverRepository_DuplicateIsTyped(t *testing.T) {
	db := openDB(t)
	repo := NewGORMCaregiverRepository(db)
	ctx := context.Background()

	petID := makePet(t, db)
	userID := uuid.New()

	if _, err := repo.Save(ctx, &domain.PetCaregiver{
		ID: uuid.New(), PetID: petID, UserID: userID,
	}); err != nil {
		t.Fatalf("ครั้งแรกต้องสำเร็จ: %v", err)
	}

	_, err := repo.Save(ctx, &domain.PetCaregiver{
		ID: uuid.New(), PetID: petID, UserID: userID,
	})
	if !errors.Is(err, domain.ErrCaregiverDuplicate) {
		t.Fatalf("ต้องเป็น ErrCaregiverDuplicate ได้ %v", err)
	}
}

func TestCaregiverRepository_FindByIDNotFound(t *testing.T) {
	db := openDB(t)
	repo := NewGORMCaregiverRepository(db)

	_, err := repo.FindByID(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrCaregiverNotFound) {
		t.Errorf("ต้องเป็น ErrCaregiverNotFound ได้ %v", err)
	}
}

// TestSetPermissions_DoesNotTouchMasterData คือเทสต์ที่สำคัญที่สุดของไฟล์นี้
//
// S-4: เดิมใช้ Association("Permissions").Replace(...) ซึ่ง GORM จะ upsert
// แถวใน pet_permissions (ตาราง master) ให้ด้วย ทำให้ client ที่ส่ง object
// เต็มก้อนมาแก้ชื่อ/คำอธิบายของ permission หรือสร้าง ID ใหม่ได้
func TestSetPermissions_DoesNotTouchMasterData(t *testing.T) {
	db := openDB(t)
	repo := NewGORMCaregiverRepository(db)
	ctx := context.Background()

	petID := makePet(t, db)
	saved, err := repo.Save(ctx, &domain.PetCaregiver{
		ID: uuid.New(), PetID: petID, UserID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("Save ไม่สำเร็จ: %v", err)
	}

	var beforeCount int64
	var beforeName string
	db.Raw("SELECT count(*) FROM pet_permissions").Scan(&beforeCount)
	db.Raw("SELECT name FROM pet_permissions WHERE id = 'MANAGE_WATER'").Scan(&beforeName)

	if _, err := repo.SetPermissions(ctx, saved.ID, []string{"MANAGE_WATER", "MANAGE_LITTER"}); err != nil {
		t.Fatalf("SetPermissions ไม่สำเร็จ: %v", err)
	}

	var afterCount int64
	var afterName string
	db.Raw("SELECT count(*) FROM pet_permissions").Scan(&afterCount)
	db.Raw("SELECT name FROM pet_permissions WHERE id = 'MANAGE_WATER'").Scan(&afterName)

	if afterCount != beforeCount {
		t.Errorf("จำนวนแถวใน master เปลี่ยนจาก %d เป็น %d", beforeCount, afterCount)
	}
	if afterName != beforeName {
		t.Errorf("ชื่อใน master เปลี่ยนจาก %q เป็น %q", beforeName, afterName)
	}

	got, err := repo.FindByID(ctx, saved.ID)
	if err != nil {
		t.Fatalf("FindByID ไม่สำเร็จ: %v", err)
	}
	if len(got.Permissions) != 2 {
		t.Errorf("ได้ %d สิทธิ์ ต้องได้ 2", len(got.Permissions))
	}
}

// TestSetPermissions_ReplacesNotAppends
func TestSetPermissions_ReplacesNotAppends(t *testing.T) {
	db := openDB(t)
	repo := NewGORMCaregiverRepository(db)
	ctx := context.Background()

	petID := makePet(t, db)
	saved, _ := repo.Save(ctx, &domain.PetCaregiver{
		ID: uuid.New(), PetID: petID, UserID: uuid.New(),
	})

	if _, err := repo.SetPermissions(ctx, saved.ID,
		[]string{"MANAGE_WATER", "MANAGE_LITTER", "EDIT_PROFILE"}); err != nil {
		t.Fatalf("ตั้งครั้งแรกไม่สำเร็จ: %v", err)
	}
	if _, err := repo.SetPermissions(ctx, saved.ID, []string{"MANAGE_WATER"}); err != nil {
		t.Fatalf("ตั้งครั้งที่สองไม่สำเร็จ: %v", err)
	}

	got, err := repo.FindByID(ctx, saved.ID)
	if err != nil {
		t.Fatalf("FindByID ไม่สำเร็จ: %v", err)
	}
	if len(got.Permissions) != 1 || got.Permissions[0].ID != "MANAGE_WATER" {
		t.Errorf("ต้องเหลือสิทธิ์เดียว ได้ %+v", got.Permissions)
	}
}

// TestSetPermissions_UnknownCaregiverIsTyped
func TestSetPermissions_UnknownCaregiverIsTyped(t *testing.T) {
	db := openDB(t)
	repo := NewGORMCaregiverRepository(db)

	_, err := repo.SetPermissions(context.Background(), uuid.New(), []string{"MANAGE_WATER"})
	if !errors.Is(err, domain.ErrCaregiverNotFound) {
		t.Errorf("ต้องเป็น ErrCaregiverNotFound ได้ %v", err)
	}
}

// TestSetPermissions_EmptyListClearsAll
func TestSetPermissions_EmptyListClearsAll(t *testing.T) {
	db := openDB(t)
	repo := NewGORMCaregiverRepository(db)
	ctx := context.Background()

	petID := makePet(t, db)
	saved, _ := repo.Save(ctx, &domain.PetCaregiver{
		ID: uuid.New(), PetID: petID, UserID: uuid.New(),
	})
	if _, err := repo.SetPermissions(ctx, saved.ID, []string{"MANAGE_WATER"}); err != nil {
		t.Fatalf("ตั้งสิทธิ์ไม่สำเร็จ: %v", err)
	}

	if _, err := repo.SetPermissions(ctx, saved.ID, nil); err != nil {
		t.Fatalf("ล้างสิทธิ์ไม่สำเร็จ: %v", err)
	}
	got, _ := repo.FindByID(ctx, saved.ID)
	if len(got.Permissions) != 0 {
		t.Errorf("ต้องไม่เหลือสิทธิ์ ได้ %+v", got.Permissions)
	}
}

func TestCaregiverRepository_Delete(t *testing.T) {
	db := openDB(t)
	repo := NewGORMCaregiverRepository(db)
	ctx := context.Background()

	petID := makePet(t, db)
	saved, _ := repo.Save(ctx, &domain.PetCaregiver{
		ID: uuid.New(), PetID: petID, UserID: uuid.New(),
	})

	if err := repo.Delete(ctx, saved.ID); err != nil {
		t.Fatalf("Delete ไม่สำเร็จ: %v", err)
	}
	if _, err := repo.FindByID(ctx, saved.ID); !errors.Is(err, domain.ErrCaregiverNotFound) {
		t.Errorf("ลบแล้วต้องหาไม่เจอ ได้ %v", err)
	}
}

func TestPermissionRepository_FindAll(t *testing.T) {
	db := openDB(t)
	repo := NewGORMPermissionRepository(db)

	all, err := repo.FindAll(context.Background())
	if err != nil {
		t.Fatalf("FindAll ไม่สำเร็จ: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("master data ของ permission ต้องมีข้อมูล")
	}
	for _, p := range all {
		if p.ID == "" {
			t.Error("permission ต้องมี ID")
		}
	}
}
