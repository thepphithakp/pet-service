package bootstrap

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/config"
	"github.com/vertex/pet-service/pkg/middleware"
)

// publicKeyPEM สร้างคีย์ใหม่แล้วคืน PEM ของ public key
func publicKeyPEM(t *testing.T) (string, *rsa.PublicKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("สร้างคีย์ไม่สำเร็จ: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("แปลง public key ไม่สำเร็จ: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), &key.PublicKey
}

// TestLoadPublicKeys_SourcePriority ลำดับที่อ่านต้องคงที่
//
// ผิดลำดับแล้วอาจไปใช้คีย์เก่าในไฟล์ image ทั้งที่ตั้ง Secret ใหม่ไว้แล้ว
// ซึ่งทำให้ token ที่ auth-service เพิ่งออกใช้ไม่ได้ทั้งระบบ
func TestLoadPublicKeys_SourcePriority(t *testing.T) {
	fromKeys, keyA := publicKeyPEM(t)
	fromKey, keyB := publicKeyPEM(t)
	fromFile, keyC := publicKeyPEM(t)

	path := filepath.Join(t.TempDir(), "public.pem")
	if err := os.WriteFile(path, []byte(fromFile), 0o600); err != nil {
		t.Fatalf("เขียนไฟล์ทดสอบไม่ได้: %v", err)
	}

	cases := []struct {
		name string
		cfg  config.JWTConfig
		want *rsa.PublicKey
	}{
		{
			"JWT_PUBLIC_KEYS มาก่อนทุกอย่าง",
			config.JWTConfig{PublicKeysPEM: fromKeys, PublicKeyPEM: fromKey, PublicKeyPath: path},
			keyA,
		},
		{
			"ไม่มี KEYS → ใช้ JWT_PUBLIC_KEY",
			config.JWTConfig{PublicKeyPEM: fromKey, PublicKeyPath: path},
			keyB,
		},
		{
			"ไม่มีทั้งคู่ → อ่านไฟล์",
			config.JWTConfig{PublicKeyPath: path},
			keyC,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keys, err := LoadPublicKeys(tc.cfg)
			if err != nil {
				t.Fatalf("ไม่ควร error: %v", err)
			}
			if len(keys) != 1 {
				t.Fatalf("ได้ %d คีย์ ต้องได้ 1", len(keys))
			}
			if middleware.KeyID(keys[0]) != middleware.KeyID(tc.want) {
				t.Error("อ่านคีย์มาจากแหล่งที่ผิดลำดับ")
			}
		})
	}
}

// TestLoadPublicKeys_MultipleKeysForRotation
//
// ระหว่าง rotate ต้องรับได้หลายใบพร้อมกัน ไม่งั้น token ที่ยังไม่หมดอายุ
// จะใช้ไม่ได้ทันทีที่เปลี่ยนคีย์
func TestLoadPublicKeys_MultipleKeysForRotation(t *testing.T) {
	a, keyA := publicKeyPEM(t)
	b, keyB := publicKeyPEM(t)

	keys, err := LoadPublicKeys(config.JWTConfig{PublicKeysPEM: a + b})
	if err != nil {
		t.Fatalf("ไม่ควร error: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("ได้ %d คีย์ ต้องได้ 2", len(keys))
	}

	got := map[string]bool{
		middleware.KeyID(keys[0]): true,
		middleware.KeyID(keys[1]): true,
	}
	for _, want := range []*rsa.PublicKey{keyA, keyB} {
		if !got[middleware.KeyID(want)] {
			t.Error("คีย์ที่ใส่ไว้หายไปหนึ่งใบ")
		}
	}
}

// TestLoadPublicKeys_RefusesBadInput
//
// ตั้งค่าผิดต้องล้มตั้งแต่ตอน start พร้อมบอกว่าอ่านจากแหล่งไหน
// ไม่ใช่ start ขึ้นแล้วปฏิเสธ token ทุกใบโดยไม่มีใครรู้สาเหตุ
func TestLoadPublicKeys_RefusesBadInput(t *testing.T) {
	cases := []struct {
		name      string
		cfg       config.JWTConfig
		wantInMsg string
	}{
		{"ไม่ใช่ PEM", config.JWTConfig{PublicKeysPEM: "ไม่ใช่คีย์"}, "JWT_PUBLIC_KEYS"},
		{"PEM ที่เนื้อในเสีย", config.JWTConfig{
			PublicKeysPEM: "-----BEGIN PUBLIC KEY-----\nQUJD\n-----END PUBLIC KEY-----\n",
		}, "JWT_PUBLIC_KEYS"},
		{"ไฟล์ไม่มีอยู่", config.JWTConfig{PublicKeyPath: "/ไม่มี/ไฟล์นี้.pem"}, "ไม่มี/ไฟล์นี้.pem"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadPublicKeys(tc.cfg)
			if err == nil {
				t.Fatal("ต้องคืน error")
			}
			if !strings.Contains(err.Error(), tc.wantInMsg) {
				t.Errorf("ข้อความต้องบอกแหล่งที่มา %q: %v", tc.wantInMsg, err)
			}
		})
	}
}

// TestNewAuthConfig_AdminUserIDs
func TestNewAuthConfig_AdminUserIDs(t *testing.T) {
	pemStr, _ := publicKeyPEM(t)
	admin := uuid.New()

	t.Run("uuid ถูกต้อง", func(t *testing.T) {
		cfg, err := NewAuthConfig(config.JWTConfig{
			PublicKeysPEM: pemStr,
			AdminUserIDs:  []string{admin.String()},
		})
		if err != nil {
			t.Fatalf("ไม่ควร error: %v", err)
		}
		if !cfg.AdminUserIDs[admin] {
			t.Error("uuid ที่ตั้งไว้ต้องอยู่ในรายการ admin")
		}
	})

	t.Run("uuid ผิดต้องล้มตั้งแต่ start", func(t *testing.T) {
		_, err := NewAuthConfig(config.JWTConfig{
			PublicKeysPEM: pemStr,
			AdminUserIDs:  []string{"ไม่ใช่ uuid"},
		})
		if err == nil {
			t.Fatal("ต้องคืน error — ปล่อยผ่านแล้วจะเงียบจนไม่มีใครรู้ว่าตั้งค่าผิด")
		}
		if !strings.Contains(err.Error(), "ADMIN_USER_IDS") {
			t.Errorf("ข้อความต้องบอกว่าเป็นค่าไหน: %v", err)
		}
	})

	t.Run("ส่งต่อ issuer และ audience", func(t *testing.T) {
		cfg, err := NewAuthConfig(config.JWTConfig{
			PublicKeysPEM: pemStr,
			Issuer:        "vertex-auth",
			Audience:      "vertex-app",
		})
		if err != nil {
			t.Fatalf("ไม่ควร error: %v", err)
		}
		if cfg.Issuer != "vertex-auth" || cfg.Audience != "vertex-app" {
			t.Error("issuer/audience ต้องถูกส่งต่อไปยัง middleware")
		}
	})
}
