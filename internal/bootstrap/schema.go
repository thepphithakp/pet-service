package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// RequiredSchemaVersion คือเวอร์ชัน migration ต่ำสุดที่โค้ดชุดนี้ทำงานได้
//
// ⚠️ ต้องเพิ่มค่านี้ทุกครั้งที่เพิ่ม V__ ใหม่ที่โค้ดพึ่งพา
// และห้ามเพิ่มถ้าโค้ดยังไม่ได้ใช้ schema นั้น (จะทำให้ deploy ไม่ผ่านโดยไม่จำเป็น)
const RequiredSchemaVersion = 8

// ErrSchemaOutdated เกิดเมื่อ migration ยังไม่ถูกรันหรือรันไม่ครบ
var ErrSchemaOutdated = errors.New("schema version ต่ำกว่าที่โค้ดต้องการ")

// AssertSchemaVersion มาแทน AutoMigrate
//
// เดิม app จัดการ schema เองตอน start ซึ่ง:
//   - race เมื่อ replica > 1
//   - ลบ/rename column ไม่ได้ ทำ data migration ไม่ได้
//   - review ใน PR ไม่ได้
//
// ตอนนี้ Flyway Job รันก่อน pod ใหม่ขึ้น (helm pre-upgrade hook)
// หน้าที่ของ app เหลือแค่ "ยืนยันว่า migration รันแล้วจริง" แล้วล้มทันทีถ้ายัง
// เพื่อไม่ให้ไปพังตอน query แรกด้วย error ที่อ่านไม่รู้เรื่อง
func AssertSchemaVersion(ctx context.Context, db *gorm.DB) error {
	var version string
	err := db.WithContext(ctx).Raw(`
		SELECT version
		FROM flyway_schema_history
		WHERE success AND version IS NOT NULL
		ORDER BY installed_rank DESC
		LIMIT 1`).Scan(&version).Error
	if err != nil {
		return fmt.Errorf(
			"อ่านตาราง flyway_schema_history ไม่ได้ — migration อาจยังไม่เคยรัน "+
				"(ตรวจ search_path และ Flyway Job): %w", err)
	}
	if version == "" {
		return fmt.Errorf("%w: ไม่พบ migration ที่สำเร็จเลย (ต้องการอย่างน้อย V%d)",
			ErrSchemaOutdated, RequiredSchemaVersion)
	}

	major, err := majorVersion(version)
	if err != nil {
		return fmt.Errorf("อ่าน schema version %q ไม่ได้: %w", version, err)
	}
	if major < RequiredSchemaVersion {
		return fmt.Errorf("%w: ฐานข้อมูลอยู่ที่ V%s แต่โค้ดต้องการ V%d — รัน migration ก่อน",
			ErrSchemaOutdated, version, RequiredSchemaVersion)
	}
	return nil
}

// majorVersion อ่านเลขหน้าสุดของ version string ("8" → 8, "8.1" → 8)
func majorVersion(v string) (int, error) {
	head, _, _ := strings.Cut(strings.TrimSpace(v), ".")
	return strconv.Atoi(head)
}
