package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func sign(t *testing.T, k *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(k)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func authApp(pub *rsa.PublicKey) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Get("/x", NewAuthMiddleware(pub), func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"userId":   c.Locals("userId"),
			"userName": c.Locals("userName"),
		})
	})
	return app
}

func TestAuthMiddleware(t *testing.T) {
	key := testKey(t)
	app := authApp(&key.PublicKey)

	t.Run("token ถูกต้อง → ผ่าน และเซ็ต locals", func(t *testing.T) {
		tok := sign(t, key, jwt.MapClaims{
			"sub":  "11111111-1111-1111-1111-111111111111",
			"name": "เทพ",
			"exp":  time.Now().Add(time.Hour).Unix(),
		})
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		res, _ := app.Test(req, -1)
		if res.StatusCode != 200 {
			t.Fatalf("status = %d", res.StatusCode)
		}
		b, _ := io.ReadAll(res.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		if m["userId"] != "11111111-1111-1111-1111-111111111111" || m["userName"] != "เทพ" {
			t.Fatalf("locals ไม่ตรง: %s", b)
		}
	})

	t.Run("ไม่มี header → 401", func(t *testing.T) {
		res, _ := app.Test(httptest.NewRequest("GET", "/x", nil), -1)
		if res.StatusCode != 401 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("token หมดอายุ → 401", func(t *testing.T) {
		tok := sign(t, key, jwt.MapClaims{"sub": "u", "exp": time.Now().Add(-time.Hour).Unix()})
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		res, _ := app.Test(req, -1)
		if res.StatusCode != 401 {
			t.Fatalf("token หมดอายุต้องได้ 401, ได้ %d", res.StatusCode)
		}
	})

	t.Run("เซ็นด้วย key อื่น → 401", func(t *testing.T) {
		other := testKey(t)
		tok := sign(t, other, jwt.MapClaims{"sub": "u", "exp": time.Now().Add(time.Hour).Unix()})
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		res, _ := app.Test(req, -1)
		if res.StatusCode != 401 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("อัลกอริทึม none → 401 (กัน alg confusion)", func(t *testing.T) {
		tok, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
			"sub": "u", "exp": time.Now().Add(time.Hour).Unix(),
		}).SignedString(jwt.UnsafeAllowNoneSignatureType)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		res, _ := app.Test(req, -1)
		if res.StatusCode != 401 {
			t.Fatalf("alg=none ต้องถูกปฏิเสธ, ได้ %d", res.StatusCode)
		}
	})

	t.Run("ไม่มี sub → 401", func(t *testing.T) {
		tok := sign(t, key, jwt.MapClaims{"name": "x", "exp": time.Now().Add(time.Hour).Unix()})
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		res, _ := app.Test(req, -1)
		if res.StatusCode != 401 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	// S-5: ยังไม่ verify iss/aud — token จาก issuer อื่นที่เซ็นด้วย key เดียวกันก็ผ่าน
	t.Run("known gap S-5: ไม่ verify iss/aud", func(t *testing.T) {
		tok := sign(t, key, jwt.MapClaims{
			"sub": "11111111-1111-1111-1111-111111111111",
			"iss": "https://issuer-ปลอม",
			"aud": "service-อื่น",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		res, _ := app.Test(req, -1)
		if res.StatusCode == 200 {
			t.Log("ยืนยัน S-5: iss/aud ไม่ถูกตรวจ — Phase 1.5 ต้องเปิด validate (แบบ 2 เฟส)")
			return
		}
		t.Log("S-5 ถูกแก้แล้ว")
	})
}
