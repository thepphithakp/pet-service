package bootstrap

import (
	"crypto/rsa"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/config"
	"github.com/vertex/pet-service/pkg/middleware"
)

// LoadPublicKeys อ่าน public key ทุกใบที่ยอมรับได้
//
// ลำดับความสำคัญ:
//  1. JWT_PUBLIC_KEYS — PEM หลายบล็อกต่อกันใน env เดียว (รูปแบบที่ใช้ตอน rotate)
//  2. JWT_PUBLIC_KEY  — PEM ใบเดียว (รูปแบบเดิม)
//  3. ไฟล์ใน image    — พฤติกรรมดั้งเดิม
//
// PEM เป็นรูปแบบที่ระบุขอบเขตของตัวเองอยู่แล้ว จึงต่อกันหลายบล็อกใน env
// ตัวเดียวได้โดยไม่ต้องมีตัวคั่นพิเศษ ทำให้ใส่ผ่าน Secret ได้ตรงไปตรงมา
func LoadPublicKeys(cfg config.JWTConfig) ([]*rsa.PublicKey, error) {
	raw := cfg.PublicKeysPEM
	source := "JWT_PUBLIC_KEYS"

	if raw == "" {
		raw, source = cfg.PublicKeyPEM, "JWT_PUBLIC_KEY"
	}
	if raw == "" {
		b, err := os.ReadFile(cfg.PublicKeyPath)
		if err != nil {
			return nil, fmt.Errorf("อ่าน public key จาก %s ไม่ได้: %w", cfg.PublicKeyPath, err)
		}
		raw, source = string(b), cfg.PublicKeyPath
	}

	keys, err := parsePublicKeys(raw)
	if err != nil {
		return nil, fmt.Errorf("อ่าน public key จาก %s ไม่สำเร็จ: %w", source, err)
	}

	for _, k := range keys {
		slog.Info("ยอมรับ public key", "kid", middleware.KeyID(k), "source", source)
	}
	if len(keys) > 1 {
		slog.Warn("ตั้งค่า public key ไว้หลายใบ — โหมดนี้ใช้ระหว่าง rotate key เท่านั้น "+
			"เอาใบเก่าออกเมื่อผ่านไปนานกว่าอายุ token ที่ยาวที่สุด", "count", len(keys))
	}
	return keys, nil
}

// parsePublicKeys แยก PEM หลายบล็อกออกจากกันแล้ว parse ทีละใบ
func parsePublicKeys(raw string) ([]*rsa.PublicKey, error) {
	var keys []*rsa.PublicKey
	rest := []byte(raw)

	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		key, err := jwt.ParseRSAPublicKeyFromPEM(pem.EncodeToMemory(block))
		if err != nil {
			return nil, fmt.Errorf("parse PEM block %q ไม่สำเร็จ: %w", block.Type, err)
		}
		keys = append(keys, key)
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("ไม่พบ PEM block ที่ใช้ได้")
	}
	return keys, nil
}

// NewAuthConfig ประกอบค่าตั้งต้นของ auth middleware
func NewAuthConfig(cfg config.JWTConfig) (middleware.AuthConfig, error) {
	keys, err := LoadPublicKeys(cfg)
	if err != nil {
		return middleware.AuthConfig{}, err
	}

	adminIDs := make(map[uuid.UUID]bool, len(cfg.AdminUserIDs))
	for _, rawID := range cfg.AdminUserIDs {
		id, err := uuid.Parse(rawID)
		if err != nil {
			return middleware.AuthConfig{}, fmt.Errorf("ADMIN_USER_IDS มี uuid ที่ไม่ถูกต้อง %q: %w", rawID, err)
		}
		adminIDs[id] = true
	}
	if len(adminIDs) > 0 {
		slog.Warn("ADMIN_USER_IDS ถูกตั้งไว้ — เป็นสะพานชั่วคราวระหว่างรอ roles claim "+
			"จาก auth-service ถอดออกเมื่อพร้อม", "count", len(adminIDs))
	}

	return middleware.AuthConfig{
		PublicKeys:   keys,
		Issuer:       cfg.Issuer,
		Audience:     cfg.Audience,
		AdminUserIDs: adminIDs,
	}, nil
}
