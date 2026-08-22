//go:build integration

package bootstrap

import (
	"context"
	"os"
	"sync"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/vertex/pet-service/internal/adapter/repository/model"
)

// openTestDB ต่อกับฐานข้อมูลที่รัน Flyway migration มาแล้ว
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ไม่ได้ตั้ง TEST_DATABASE_URL — ข้าม integration test (ใช้ make db-up ก่อน)")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("ต่อฐานข้อมูลไม่ได้: %v", err)
	}
	assertDisposableDB(t, db)
	return db
}

// assertDisposableDB กันไม่ให้ integration test (ซึ่งเขียนและลบข้อมูล)
// ไปรันใส่ฐานข้อมูลจริงโดยไม่ตั้งใจ
//
// เคสที่เกิดขึ้นจริง: มี kubectl/k9s port-forward ของ postgres ใน cluster
// ค้างอยู่ที่ localhost:5432 ทำให้ DSN default ของ make test-integration
// ชี้ไป production แทน docker รอบนั้นรอดเพราะ role ไม่ตรงเท่านั้น
//
// ลายนิ้วมือที่ใช้คือ repeatable migration ของ seed ตัวอย่างสำหรับ dev
// (pet/seed/R__9000_dev_sample_pets.sql) ซึ่ง FLYWAY_LOCATIONS ของ prod
// ไม่เคยโหลด จึงมีได้เฉพาะฐานข้อมูลที่ยกมาเพื่อ dev/test เท่านั้น
func assertDisposableDB(t *testing.T, db *gorm.DB) {
	t.Helper()

	const devSeedDescription = "9000 dev sample pets"

	var n int64
	err := db.Raw(
		`SELECT count(*) FROM flyway_schema_history WHERE description = ? AND success`,
		devSeedDescription,
	).Scan(&n).Error
	if err != nil {
		t.Fatalf("ตรวจ flyway_schema_history ไม่ได้ (ฐานข้อมูลนี้อาจไม่ใช่ของ dev): %v", err)
	}
	if n == 0 {
		var dbName, user string
		db.Raw("SELECT current_database()").Scan(&dbName)
		db.Raw("SELECT current_user").Scan(&user)
		t.Fatalf(
			"ปฏิเสธการรัน integration test: ฐานข้อมูล %q (user %q) ไม่มี seed ของ dev "+
				"(%q) จึงถือว่าไม่ใช่ฐานข้อมูลชั่วคราวที่ทิ้งได้\n"+
				"ถ้าตั้งใจรัน local ให้ `make db-reset` แล้วตรวจว่าไม่มี port-forward "+
				"ของ postgres ตัวอื่นทับพอร์ตอยู่ (lsof -nP -iTCP:5432 -sTCP:LISTEN)",
			dbName, user, devSeedDescription,
		)
	}
}

// TestSchemaMatchesModels คือ safety net ที่มาแทน AutoMigrate
//
// เมื่อไม่มี AutoMigrate แล้ว GORM model กับไฟล์ SQL จะไม่ sync กันเองอีกต่อไป
// ใครเพิ่ม field ใน model แล้วลืมเขียน migration จะรู้ตัวตอน CI ไม่ใช่ตอน production
func TestSchemaMatchesModels(t *testing.T) {
	db := openTestDB(t)
	m := db.Migrator()

	models := []any{
		&model.Pet{}, &model.Permission{}, &model.Caregiver{},
		&model.Litter{}, &model.Water{},
	}

	for _, mdl := range models {
		s, err := schema.Parse(mdl, &sync.Map{}, db.NamingStrategy)
		if err != nil {
			t.Fatalf("parse schema: %v", err)
		}

		if !m.HasTable(mdl) {
			t.Errorf("ไม่พบตาราง %s — migration ขาดหรือ search_path ผิด", s.Table)
			continue
		}

		for _, f := range s.Fields {
			// ข้าม field ที่ไม่ใช่ column จริง เช่น relation
			if f.DBName == "" || f.IgnoreMigration {
				continue
			}
			if !m.HasColumn(mdl, f.DBName) {
				t.Errorf("ตาราง %s ไม่มี column %q (field %s) — เพิ่ม field ใน model แล้วลืมเขียน migration?",
					s.Table, f.DBName, f.Name)
			}
		}
	}
}

// TestSchemaJoinTableExists ตรวจ join table ของ many2many
// ซึ่ง schema.Parse ตรวจให้ไม่ได้เพราะไม่ใช่ model โดยตรง
func TestSchemaJoinTableExists(t *testing.T) {
	db := openTestDB(t)
	for _, col := range []string{"caregiver_model_id", "permission_model_id"} {
		var exists bool
		err := db.Raw(`
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'caregiver_permissions' AND column_name = ?
			)`, col).Scan(&exists).Error
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("caregiver_permissions ต้องมี column %q — ดู model/schema_test.go ว่าทำไมชื่อนี้ห้ามเปลี่ยน", col)
		}
	}
}

// TestSchemaVersionGuard ตรวจว่า guard อ่าน flyway_schema_history ได้จริง
func TestSchemaVersionGuard(t *testing.T) {
	db := openTestDB(t)
	if err := AssertSchemaVersion(context.Background(), db); err != nil {
		t.Fatalf("AssertSchemaVersion ล้มเหลว: %v", err)
	}
}

// TestMasterDataSeeded ตรวจว่า V3 seed ค่าที่ API v1 ต้องคืนครบ
// legacy_label ต้องตรงกับที่ litter_service.go เคย hardcode ไว้ทุกตัวอักษร
func TestMasterDataSeeded(t *testing.T) {
	db := openTestDB(t)

	var breeds []string
	if err := db.Raw(`SELECT legacy_label FROM mst_cat_breeds
	                  WHERE is_active ORDER BY sort_order`).Scan(&breeds).Error; err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Scottish Fold (หูพับ)", "Scottish Straight (หูตั้ง)", "British Shorthair",
		"Persian", "Maine Coon", "Siamese (วิเชียรมาศ)", "Khao Manee (ขาวมณี)",
		"Sphynx", "Bengal", "Ragdoll", "American Shorthair",
		"Exotic Shorthair", "Munchkin (ขาสั้น)", "Mixed / Other (พันธุ์ผสม/อื่นๆ)",
	}
	if len(breeds) != len(want) {
		t.Fatalf("จำนวนสายพันธุ์ = %d ต้องการ %d: %v", len(breeds), len(want), breeds)
	}
	for i := range want {
		if breeds[i] != want[i] {
			t.Errorf("breeds[%d] = %q ต้องการ %q — API v1 จะคืนค่าไม่เหมือนเดิม", i, breeds[i], want[i])
		}
	}

	var perms int64
	db.Raw(`SELECT count(*) FROM pet_permissions WHERE is_active`).Scan(&perms)
	if perms != 6 {
		t.Errorf("permission ที่ active = %d ต้องการ 6 (รวม MANAGE_WATER ที่เพิ่มใน V7)", perms)
	}

	var caps int64
	db.Raw(`SELECT count(*) FROM role_capabilities WHERE role_code = 'SUPER_ADMIN'`).Scan(&caps)
	if caps < 7 {
		t.Errorf("SUPER_ADMIN capability = %d ต้องการอย่างน้อย 7", caps)
	}
}
