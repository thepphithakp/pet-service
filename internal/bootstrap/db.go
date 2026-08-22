package bootstrap

import (
	"fmt"
	"log"
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
		log.Printf("เชื่อมต่อ DB ครั้งที่ %d ไม่สำเร็จ (%s): %v — ลองใหม่ใน %s",
			i+1, cfg.Redacted(), err, dbConnectBackoff)
		time.Sleep(dbConnectBackoff)
	}
	return nil, fmt.Errorf("เชื่อมต่อฐานข้อมูลไม่สำเร็จหลังลอง %d ครั้ง: %w", dbConnectAttempts, err)
}
