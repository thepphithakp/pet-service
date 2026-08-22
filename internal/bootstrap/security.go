package bootstrap

import (
	"crypto/rsa"
	"fmt"
	"os"

	"github.com/golang-jwt/jwt/v5"

	"github.com/vertex/pet-service/internal/config"
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
