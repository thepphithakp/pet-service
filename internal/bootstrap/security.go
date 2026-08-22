package bootstrap

import (
	"crypto/rsa"
	"fmt"
	"log"
	"os"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/config"
	"github.com/vertex/pet-service/pkg/middleware"
)

// LoadPublicKey อ่าน RSA public key สำหรับ verify JWT
// รับจาก env ก่อน (JWT_PUBLIC_KEY) แล้วค่อย fallback ไปอ่านไฟล์
// เพื่อให้ rotate key ผ่าน Secret ได้โดยไม่ต้อง rebuild image (S-6)
func LoadPublicKey(cfg config.JWTConfig) (*rsa.PublicKey, error) {
	pem := []byte(cfg.PublicKeyPEM)

	if len(pem) == 0 {
		var err error
		pem, err = os.ReadFile(cfg.PublicKeyPath)
		if err != nil {
			return nil, fmt.Errorf("อ่าน public key จาก %s ไม่ได้: %w", cfg.PublicKeyPath, err)
		}
	}

	key, err := jwt.ParseRSAPublicKeyFromPEM(pem)
	if err != nil {
		return nil, fmt.Errorf("parse RSA public key ไม่สำเร็จ: %w", err)
	}
	return key, nil
}

// NewAuthConfig ประกอบค่าตั้งต้นของ auth middleware
func NewAuthConfig(cfg config.JWTConfig) (middleware.AuthConfig, error) {
	key, err := LoadPublicKey(cfg)
	if err != nil {
		return middleware.AuthConfig{}, err
	}

	adminIDs := make(map[uuid.UUID]bool, len(cfg.AdminUserIDs))
	for _, raw := range cfg.AdminUserIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return middleware.AuthConfig{}, fmt.Errorf("ADMIN_USER_IDS มี uuid ที่ไม่ถูกต้อง %q: %w", raw, err)
		}
		adminIDs[id] = true
	}
	if len(adminIDs) > 0 {
		log.Printf("⚠️  ADMIN_USER_IDS ตั้งไว้ %d รายการ — เป็นสะพานชั่วคราวระหว่างรอ roles claim จาก auth-service ถอดออกเมื่อพร้อม", len(adminIDs))
	}

	return middleware.AuthConfig{
		PublicKey:    key,
		Issuer:       cfg.Issuer,
		Audience:     cfg.Audience,
		AdminUserIDs: adminIDs,
	}, nil
}
