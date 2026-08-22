// Package config รวมการอ่านค่าตั้งค่าจาก environment ไว้ที่เดียว
// แก้ A-5: เดิม os.Getenv กระจายอยู่ใน cmd/server/main.go และ adapter/event/http_publisher.go
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Port string

	DB  DBConfig
	JWT JWTConfig

	EventServiceURL string

	Log      LogConfig
	Shutdown ShutdownConfig
}

// ShutdownConfig คุมพฤติกรรมตอนปิดตัว
type ShutdownConfig struct {
	// DrainDelay คือเวลารอหลังปิด readiness ก่อนหยุดรับ connection ใหม่
	//
	// ต้องมากกว่ารอบของ readinessProbe (periodSeconds × failureThreshold)
	// ไม่งั้นจะปิด listener ก่อนที่ k8s จะถอด endpoint เสร็จ
	// แล้ว request ที่ยังถูกส่งมาจะโดน connection refused
	DrainDelay time.Duration
	// Timeout คือเวลาสูงสุดที่รอ request ที่ค้างอยู่ให้ทำงานจนจบ
	//
	// ต้องน้อยกว่า terminationGracePeriodSeconds ของ pod
	// ไม่งั้น k8s จะ SIGKILL ก่อนที่จะปิดเสร็จ
	Timeout time.Duration
}

type LogConfig struct {
	Level string
	// Body เปิดการ log request/response body — ห้ามเปิดบน production
	Body bool
}

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
	TimeZone string

	// SearchPath กำหนด schema ที่ใช้ค้นหาตาราง
	//
	// ⚠️ ระหว่างย้ายตารางจาก public ไป pet ต้องตั้งเป็น "pet,public" และ
	//    deploy app ตัวนี้ให้ขึ้นก่อน แล้วค่อยรัน db/bootstrap/001_move_to_pet_schema.sql
	//    ถ้าย้ายก่อนที่ app จะรู้จัก schema pet จะได้
	//    relation "pets" does not exist ทั้งระบบทันที
	//
	//    เมื่อ auth-service และ event-service ย้าย schema เสร็จแล้ว
	//    ค่อยเปลี่ยนเป็น "pet" อย่างเดียว
	SearchPath string
}

type JWTConfig struct {
	// PublicKeyPEM มาก่อน PublicKeyPath เสมอ — ทำให้ rotate key ผ่าน Secret ได้
	// โดยไม่ต้อง rebuild image (S-6)
	PublicKeyPEM  string
	PublicKeyPath string

	// PublicKeysPEM รับ PEM หลายบล็อกต่อกันในค่าเดียว
	//
	// ใช้ตอน rotate key: ระหว่างเปลี่ยนผ่านต้องยอมรับทั้งใบเก่าและใบใหม่
	// จนกว่า token ที่เซ็นด้วยใบเก่าจะหมดอายุครบทุกใบ
	PublicKeysPEM string

	// Issuer / Audience ตรวจเมื่อไม่ว่างเท่านั้น
	// ⚠️ เปิดได้หลัง auth-service ออก claim สองตัวนี้แล้วอย่างน้อย 72 ชม.
	//    (เท่าอายุ token เดิม) ไม่งั้น token ที่ผู้ใช้ถืออยู่จะใช้ไม่ได้ทันที
	Issuer   string
	Audience string

	// AdminUserIDs สะพานชั่วคราวระหว่างรอ roles claim จาก Phase 1A
	// user id ในรายการนี้จะได้ SUPER_ADMIN — ถอดออกเมื่อ auth-service พร้อม
	AdminUserIDs []string
}

// DSN คืน connection string สำหรับ gorm postgres driver
func (d DBConfig) DSN() string {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s",
		d.Host, d.User, d.Password, d.Name, d.Port, d.SSLMode, d.TimeZone,
	)
	if d.SearchPath != "" {
		dsn += " search_path=" + d.SearchPath
	}
	return dsn
}

// Redacted คืน DSN ที่ซ่อนรหัสผ่าน สำหรับใส่ใน log
func (d DBConfig) Redacted() string {
	return fmt.Sprintf("host=%s user=%s dbname=%s port=%s sslmode=%s search_path=%s",
		d.Host, d.User, d.Name, d.Port, d.SSLMode, d.SearchPath)
}

// Load อ่านค่าจาก environment พร้อม default ที่ตรงกับพฤติกรรมเดิมทุกตัว
func Load() (Config, error) {
	cfg := Config{
		Port: env("PORT", "4001"),
		DB: DBConfig{
			Host:     os.Getenv("DB_HOST"),
			Port:     os.Getenv("DB_PORT"),
			User:     os.Getenv("DB_USER"),
			Password: os.Getenv("DB_PASSWORD"),
			Name:     os.Getenv("DB_NAME"),
			SSLMode:  env("DB_SSLMODE", "disable"),
			TimeZone: env("DB_TIMEZONE", "Asia/Bangkok"),
			// default "pet,public" เพื่อให้ deploy ได้ทั้งก่อนและหลังย้าย schema
			SearchPath: env("DB_SEARCH_PATH", "pet,public"),
		},
		JWT: JWTConfig{
			PublicKeyPEM:  os.Getenv("JWT_PUBLIC_KEY"),
			PublicKeyPath: env("JWT_PUBLIC_KEY_PATH", "keys/public.pem"),
			PublicKeysPEM: os.Getenv("JWT_PUBLIC_KEYS"),
			Issuer:        os.Getenv("JWT_ISSUER"),
			Audience:      os.Getenv("JWT_AUDIENCE"),
			AdminUserIDs:  splitList(os.Getenv("ADMIN_USER_IDS")),
		},
		EventServiceURL: env("EVENT_SERVICE_URL", "http://event-service.vertex.svc.cluster.local:4002"),
		Log: LogConfig{
			Level: env("LOG_LEVEL", "info"),
			Body:  os.Getenv("LOG_BODY") == "true",
		},
		Shutdown: ShutdownConfig{
			DrainDelay: envDuration("SHUTDOWN_DRAIN_DELAY", 5*time.Second),
			Timeout:    envDuration("SHUTDOWN_TIMEOUT", 20*time.Second),
		},
	}
	return cfg, cfg.Validate()
}

// Validate ตรวจค่าที่ขาดไม่ได้ เพื่อให้ล้มตั้งแต่ตอน boot พร้อมข้อความชัดเจน
// แทนที่จะไปพังตอน query แรกหรือ retry ครบ 5 รอบแล้วค่อย log.Fatal
func (c Config) Validate() error {
	var missing []string
	for name, v := range map[string]string{
		"DB_HOST": c.DB.Host,
		"DB_PORT": c.DB.Port,
		"DB_USER": c.DB.User,
		"DB_NAME": c.DB.Name,
	} {
		if strings.TrimSpace(v) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("ไม่ได้ตั้งค่า environment ที่จำเป็น: %s", strings.Join(missing, ", "))
	}
	return nil
}

// splitList แยกค่าที่คั่นด้วย comma และตัดช่องว่าง
func splitList(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// envDuration อ่านค่าเวลาแบบ "5s" "1m" ถ้าอ่านไม่ได้ใช้ค่า default
func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
