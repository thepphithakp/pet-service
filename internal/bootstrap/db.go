package bootstrap

import (
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/vertex/pet-service/internal/config"
)

const (
	dbConnectAttempts = 5
	dbConnectBackoff  = 2 * time.Second
)

// NewDB เชื่อมต่อฐานข้อมูลพร้อม retry
// ย้ายมาจาก cmd/server/main.go connectDB() โดยเปลี่ยนจาก log.Fatal เป็นคืน error
// เพื่อให้ผู้เรียกตัดสินใจเองได้ (และเทสต์ได้)
func NewDB(cfg config.DBConfig) (*gorm.DB, error) {
	var db *gorm.DB
	var err error

	for i := 0; i < dbConnectAttempts; i++ {
		db, err = gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{})
		if err == nil {
			return db, nil
		}
		slog.Warn("เชื่อมต่อ DB ไม่สำเร็จ กำลังลองใหม่",
			"attempt", i+1, "max_attempts", dbConnectAttempts,
			"dsn", cfg.Redacted(), "retry_in", dbConnectBackoff, "error", err)
		time.Sleep(dbConnectBackoff)
	}
	return nil, fmt.Errorf("เชื่อมต่อฐานข้อมูลไม่สำเร็จหลังลอง %d ครั้ง: %w", dbConnectAttempts, err)
}

// CloseDB ปิด connection pool
//
// gorm.DB ไม่มี Close() เอง ต้องดึง *sql.DB ที่อยู่ข้างในออกมาก่อน
func CloseDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
