package bootstrap

import (
	"context"
	"log"

	"gorm.io/gorm"

	"github.com/vertex/pet-service/internal/adapter/repository"
	"github.com/vertex/pet-service/internal/adapter/repository/model"
	"github.com/vertex/pet-service/internal/domain"
)

// MigrateAndSeedLegacy คือการจัดการ schema แบบเดิมด้วย GORM AutoMigrate
//
// ⚠️ DEPRECATED — ทั้งฟังก์ชันนี้จะถูกลบใน Phase 2.5 หลัง Flyway พร้อมใช้งาน
// เหตุผลที่ต้องเลิกใช้ (ดู docs/REFACTOR_PLAN.md ข้อ 5):
//   - รันทุก pod ที่ start → race เมื่อ replica > 1
//   - ลบ/rename column ไม่ได้ ทำ data migration ไม่ได้
//   - review ใน PR ไม่ได้ และ schema จริงค่อยๆ ห่างจากสิ่งที่ควบคุมได้
//
// ยังคงไว้ตอนนี้เพราะถ้าเอาออกก่อนที่ Flyway จะพร้อม การ deploy ลง DB เปล่า
// จะไม่มีตารางเลย (Phase 4.5 เป็น pure refactor ห้ามเปลี่ยนพฤติกรรม)
func MigrateAndSeedLegacy(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&model.Pet{},
		&model.Permission{},
		&model.Caregiver{},
		&model.Litter{},
		&model.Water{},
	); err != nil {
		return err
	}

	permRepo := repository.NewGORMPermissionRepository(db)
	initialPermissions := []domain.PetPermission{
		{ID: "EDIT_PROFILE", Name: "Edit Profile", Description: "Can edit pet's basic profile details", IsActive: true},
		{ID: "MANAGE_MEDICAL", Name: "Manage Medical Records", Description: "Can view and add medical records/vaccines", IsActive: true},
		{ID: "MANAGE_WEIGHT", Name: "Update Weight Log", Description: "Can add weight records", IsActive: true},
		{ID: "MANAGE_TASKS", Name: "Manage Daily Tasks", Description: "Can view and tick off daily tasks", IsActive: true},
		{ID: "MANAGE_LITTER", Name: "Record Litter Box", Description: "Can record poop and pee events", IsActive: true},
	}
	// เดิม main.go ส่ง nil เป็น context ให้ Seed (C-6) — ใช้ Background() แทนเพื่อความปลอดภัย
	if err := permRepo.Seed(context.Background(), initialPermissions); err != nil {
		log.Println("เตือน: seed permission ไม่สำเร็จ:", err)
	}

	log.Println("Database migrated and seeded successfully.")
	return nil
}
