//go:build integration

package bootstrap

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vertex/pet-service/internal/config"
)

// dbConfigFromEnv แปลง TEST_DATABASE_URL เป็น config ที่ NewDB ใช้
func dbConfigFromEnv(t *testing.T) config.DBConfig {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ไม่ได้ตั้ง TEST_DATABASE_URL")
	}

	// postgres://user:pass@host:port/dbname?params
	rest := strings.TrimPrefix(dsn, "postgres://")
	cred, hostPart, _ := strings.Cut(rest, "@")
	user, pass, _ := strings.Cut(cred, ":")
	hostPort, dbAndParams, _ := strings.Cut(hostPart, "/")
	host, port, _ := strings.Cut(hostPort, ":")
	name, _, _ := strings.Cut(dbAndParams, "?")

	return config.DBConfig{
		Host: host, Port: port, User: user, Password: pass, Name: name,
		SSLMode: "disable", SearchPath: "pet", TimeZone: "Asia/Bangkok",
		MaxOpenConns: 5, MaxIdleConns: 2,
		ConnMaxLifetime: 30 * time.Minute, ConnMaxIdleTime: 5 * time.Minute,
	}
}

// TestNewDB_AppliesPoolSettings
//
// ค่า default ของ database/sql คือเปิดได้ไม่จำกัด ถ้า configurePool
// ไม่ถูกเรียกจริง pod เดียวอาจกิน max_connections จนทุก service ต่อไม่ได้
func TestNewDB_AppliesPoolSettings(t *testing.T) {
	cfg := dbConfigFromEnv(t)

	db, err := NewDB(cfg)
	if err != nil {
		t.Fatalf("NewDB ไม่สำเร็จ: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB(db) })

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("ดึง *sql.DB ไม่ได้: %v", err)
	}

	stats := sqlDB.Stats()
	if stats.MaxOpenConnections != cfg.MaxOpenConns {
		t.Errorf("MaxOpenConnections = %d ต้องเป็น %d — เพดานไม่ถูกตั้ง",
			stats.MaxOpenConnections, cfg.MaxOpenConns)
	}

	if err := sqlDB.PingContext(context.Background()); err != nil {
		t.Errorf("ต่อฐานข้อมูลไม่ได้: %v", err)
	}
}

// TestNewDB_FailsAfterRetries
//
// ต่อไม่ได้ต้องคืน error ที่บอกว่าลองกี่ครั้งแล้ว ไม่ใช่ค้างไปเรื่อยๆ
func TestNewDB_FailsAfterRetries(t *testing.T) {
	if testing.Short() {
		t.Skip("ข้ามเพราะต้องรอ retry ครบรอบ")
	}
	cfg := dbConfigFromEnv(t)
	cfg.Password = "รหัสผ่านผิด"

	start := time.Now()
	_, err := NewDB(cfg)
	if err == nil {
		t.Fatal("รหัสผ่านผิดต้องคืน error")
	}
	if !strings.Contains(err.Error(), "เชื่อมต่อฐานข้อมูลไม่สำเร็จ") {
		t.Errorf("ข้อความต้องบอกว่าต่อไม่สำเร็จ: %v", err)
	}
	// ต้องมีการ retry จริง ไม่ใช่ล้มทันทีครั้งเดียว
	if time.Since(start) < dbConnectBackoff {
		t.Error("ต้อง retry พร้อม backoff ไม่ใช่ล้มทันที")
	}
}

// TestCloseDB_HandlesNil
func TestCloseDB_HandlesNil(t *testing.T) {
	if err := CloseDB(nil); err != nil {
		t.Errorf("ปิด db ที่เป็น nil ต้องไม่ error: %v", err)
	}
}
