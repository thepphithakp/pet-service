package config

import (
	"strings"
	"testing"
)

func setDBEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "vertex")
	t.Setenv("DB_NAME", "vertex_pet")
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

func TestRedacted_HidesPassword(t *testing.T) {
	d := DBConfig{Host: "pg", User: "u", Password: "ลับมาก", Name: "db", Port: "5432", SSLMode: "disable"}
	if strings.Contains(d.Redacted(), "ลับมาก") {
		t.Fatal("Redacted ต้องไม่มีรหัสผ่าน")
	}
}
