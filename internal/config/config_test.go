package config

import (
	"strings"
	"testing"
	"time"
)

func setDBEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "vertex")
	t.Setenv("DB_NAME", "vertex")
}

func TestLoad_Defaults(t *testing.T) {
	setDBEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	// default ต้องตรงกับพฤติกรรมเดิมใน main.go ทุกตัว
	if cfg.Port != "4001" {
		t.Fatalf("Port = %q ต้องการ 4001", cfg.Port)
	}
	if cfg.DB.SSLMode != "disable" {
		t.Fatalf("SSLMode = %q", cfg.DB.SSLMode)
	}
	if cfg.DB.TimeZone != "Asia/Bangkok" {
		t.Fatalf("TimeZone = %q", cfg.DB.TimeZone)
	}
	if cfg.JWT.PublicKeyPath != "keys/public.pem" {
		t.Fatalf("PublicKeyPath = %q", cfg.JWT.PublicKeyPath)
	}
	if cfg.EventServiceURL != "http://event-service.vertex.svc.cluster.local:4002" {
		t.Fatalf("EventServiceURL = %q", cfg.EventServiceURL)
	}
	// default ต้องครอบทั้งก่อนและหลังย้าย schema เพื่อให้ deploy ได้โดยไม่พัง
	if cfg.DB.SearchPath != "pet,public" {
		t.Fatalf("SearchPath = %q ต้องการ pet,public", cfg.DB.SearchPath)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	t.Setenv("DB_HOST", "")
	t.Setenv("DB_PORT", "")
	t.Setenv("DB_USER", "")
	t.Setenv("DB_NAME", "")
	_, err := Load()
	if err == nil {
		t.Fatal("ต้องคืน error เมื่อขาดค่าที่จำเป็น")
	}
	for _, want := range []string{"DB_HOST", "DB_PORT", "DB_USER", "DB_NAME"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error ต้องบอกว่าขาด %s: %v", want, err)
		}
	}
}

// DSN ต้องได้รูปแบบเดิมเป๊ะ เพื่อไม่ให้การเชื่อมต่อเปลี่ยนพฤติกรรม
func TestDSN_MatchesLegacyFormat(t *testing.T) {
	d := DBConfig{
		Host: "pg", User: "u", Password: "p", Name: "db",
		Port: "5432", SSLMode: "disable", TimeZone: "Asia/Bangkok",
	}
	want := "host=pg user=u password=p dbname=db port=5432 sslmode=disable TimeZone=Asia/Bangkok"
	if got := d.DSN(); got != want {
		t.Fatalf("DSN =\n%q\nต้องการ\n%q", got, want)
	}
}

func TestDSN_WithSearchPath(t *testing.T) {
	d := DBConfig{
		Host: "pg", User: "u", Password: "p", Name: "db",
		Port: "5432", SSLMode: "disable", TimeZone: "Asia/Bangkok",
		SearchPath: "pet,public",
	}
	want := "host=pg user=u password=p dbname=db port=5432 sslmode=disable TimeZone=Asia/Bangkok search_path=pet,public"
	if got := d.DSN(); got != want {
		t.Fatalf("DSN =\n%q\nต้องการ\n%q", got, want)
	}
}

func TestRedacted_HidesPassword(t *testing.T) {
	d := DBConfig{Host: "pg", User: "u", Password: "ลับมาก", Name: "db", Port: "5432", SSLMode: "disable"}
	if strings.Contains(d.Redacted(), "ลับมาก") {
		t.Fatal("Redacted ต้องไม่มีรหัสผ่าน")
	}
}

func TestShutdownConfig(t *testing.T) {
	setDBEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Shutdown.DrainDelay != 5*time.Second {
		t.Fatalf("DrainDelay = %v ต้องการ 5s", cfg.Shutdown.DrainDelay)
	}
	if cfg.Shutdown.Timeout != 20*time.Second {
		t.Fatalf("Timeout = %v ต้องการ 20s", cfg.Shutdown.Timeout)
	}

	// ค่าที่อ่านไม่ได้ต้อง fallback ไม่ใช่กลายเป็น 0
	// ถ้าเป็น 0 จะปิด listener ทันทีโดยไม่รอ endpoint ถอด → drop request
	t.Setenv("SHUTDOWN_DRAIN_DELAY", "ไม่ใช่เวลา")
	cfg2, _ := Load()
	if cfg2.Shutdown.DrainDelay != 5*time.Second {
		t.Fatalf("ค่าผิดรูปแบบต้อง fallback เป็น 5s ได้ %v", cfg2.Shutdown.DrainDelay)
	}

	t.Setenv("SHUTDOWN_DRAIN_DELAY", "3s")
	cfg3, _ := Load()
	if cfg3.Shutdown.DrainDelay != 3*time.Second {
		t.Fatalf("DrainDelay = %v ต้องการ 3s", cfg3.Shutdown.DrainDelay)
	}
}

// TestLoad_ConnectionPoolDefaults
//
// ค่า default ของ database/sql คือเปิด connection ได้ไม่จำกัด
// ถ้าไม่ตั้งเพดาน pod เดียวตอน traffic พุ่งอาจกิน max_connections ของ postgres
// (ตั้งไว้ 100) จนทุก service ต่อไม่ได้พร้อมกัน
//
// 3 service × 20 = 60 เหลือที่ให้ Flyway Job และการต่อด้วยมือ
func TestLoad_ConnectionPoolDefaults(t *testing.T) {
	setDBEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("ไม่ควร error: %v", err)
	}

	if cfg.DB.MaxOpenConns != 20 {
		t.Errorf("MaxOpenConns = %d ต้องเป็น 20", cfg.DB.MaxOpenConns)
	}
	if cfg.DB.MaxIdleConns != 10 {
		t.Errorf("MaxIdleConns = %d ต้องเป็น 10", cfg.DB.MaxIdleConns)
	}
	if cfg.DB.MaxIdleConns > cfg.DB.MaxOpenConns {
		t.Error("MaxIdleConns ห้ามมากกว่า MaxOpenConns — database/sql จะลดให้เองเงียบๆ")
	}
	if cfg.DB.ConnMaxLifetime.Minutes() != 30 {
		t.Errorf("ConnMaxLifetime = %v ต้องเป็น 30m", cfg.DB.ConnMaxLifetime)
	}
}

// TestLoad_PoolOverrideAndGuards ค่าที่ตั้งผิดต้องไม่ทำให้ได้ 0 (= ไม่จำกัด)
func TestLoad_PoolOverrideAndGuards(t *testing.T) {
	setDBEnv(t)

	cases := []struct {
		name  string
		value string
		want  int
	}{
		{"ตั้งค่าปกติ", "35", 35},
		{"ค่าติดลบ → ใช้ default", "-5", 20},
		{"ศูนย์ → ใช้ default ไม่ใช่ unlimited", "0", 20},
		{"ไม่ใช่ตัวเลข → ใช้ default", "มาก", 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DB_MAX_OPEN_CONNS", tc.value)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("ไม่ควร error: %v", err)
			}
			if cfg.DB.MaxOpenConns != tc.want {
				t.Errorf("MaxOpenConns = %d ต้องเป็น %d", cfg.DB.MaxOpenConns, tc.want)
			}
		})
	}
}

// TestLoad_PetListIncludeAvatarDefaultsToTrue
//
// default ต้องเป็นพฤติกรรมเดิม ไม่งั้น deploy แล้วรูปหายจากแอปทันที
func TestLoad_PetListIncludeAvatarDefaultsToTrue(t *testing.T) {
	setDBEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("ไม่ควร error: %v", err)
	}
	if !cfg.PetListIncludeAvatar {
		t.Error("default ต้องเป็น true เพื่อไม่ให้แอปที่ใช้อยู่พังตอน deploy")
	}

	t.Setenv("PET_LIST_INCLUDE_AVATAR", "false")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("ไม่ควร error: %v", err)
	}
	if cfg.PetListIncludeAvatar {
		t.Error("ตั้ง false แล้วต้องปิดจริง")
	}
}
