package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
)

const testSub = "11111111-1111-1111-1111-111111111111"

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

func authApp(pub *rsa.PublicKey, opts ...func(*AuthConfig)) *fiber.App {
	cfg := AuthConfig{}
	if pub != nil {
		cfg.PublicKeys = []*rsa.PublicKey{pub}
	}
	for _, o := range opts {
		o(&cfg)
	}
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Get("/x", NewAuthMiddleware(cfg), func(c *fiber.Ctx) error {
		actor, _ := domain.ActorFromContext(c.UserContext())
		return c.JSON(fiber.Map{
			"userId":   c.Locals("userId"),
			"userName": c.Locals("userName"),
			"roles":    actor.Roles,
			"email":    actor.Email,
		})
	})
	return app
}

func bearerReq(tok string) *http.Request {
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	return req
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
		tok := sign(t, key, jwt.MapClaims{"sub": testSub, "exp": time.Now().Add(-time.Hour).Unix()})
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		res, _ := app.Test(req, -1)
		if res.StatusCode != 401 {
			t.Fatalf("token หมดอายุต้องได้ 401, ได้ %d", res.StatusCode)
		}
	})

	t.Run("เซ็นด้วย key อื่น → 401", func(t *testing.T) {
		other := testKey(t)
		tok := sign(t, other, jwt.MapClaims{"sub": testSub, "exp": time.Now().Add(time.Hour).Unix()})
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

	t.Run("scheme เป็น case-insensitive ตาม RFC 7235", func(t *testing.T) {
		tok := sign(t, key, jwt.MapClaims{"sub": testSub, "exp": time.Now().Add(time.Hour).Unix()})
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("Authorization", "bearer "+tok)
		res, _ := app.Test(req, -1)
		if res.StatusCode != 200 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("sub ที่ไม่ใช่ uuid → 401", func(t *testing.T) {
		tok := sign(t, key, jwt.MapClaims{"sub": "ไม่ใช่ uuid", "exp": time.Now().Add(time.Hour).Unix()})
		res, _ := app.Test(bearerReq(tok), -1)
		if res.StatusCode != 401 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})
}

// S-5 แก้แล้ว: iss/aud ถูกตรวจเมื่อตั้งค่าไว้
func TestAuthMiddleware_IssuerAudience(t *testing.T) {
	key := testKey(t)
	strict := authApp(&key.PublicKey, func(c *AuthConfig) {
		c.Issuer = "https://auth.vertex.local"
		c.Audience = "vertex-api"
	})

	t.Run("iss/aud ถูกต้อง → ผ่าน", func(t *testing.T) {
		tok := sign(t, key, jwt.MapClaims{
			"sub": testSub, "iss": "https://auth.vertex.local", "aud": "vertex-api",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		if res, _ := strict.Test(bearerReq(tok), -1); res.StatusCode != 200 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("issuer ปลอม → 401", func(t *testing.T) {
		tok := sign(t, key, jwt.MapClaims{
			"sub": testSub, "iss": "https://issuer-ปลอม", "aud": "vertex-api",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		if res, _ := strict.Test(bearerReq(tok), -1); res.StatusCode != 401 {
			t.Fatalf("status = %d ต้องการ 401", res.StatusCode)
		}
	})

	t.Run("audience ผิด → 401", func(t *testing.T) {
		tok := sign(t, key, jwt.MapClaims{
			"sub": testSub, "iss": "https://auth.vertex.local", "aud": "service-อื่น",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		if res, _ := strict.Test(bearerReq(tok), -1); res.StatusCode != 401 {
			t.Fatalf("status = %d ต้องการ 401", res.StatusCode)
		}
	})

	// ⚠️ token เดิมที่ยังไม่มี iss/aud จะใช้ไม่ได้ทันทีถ้าเปิดค่านี้
	// เป็นเหตุผลที่ต้องปล่อยแบบ 2 เฟส
	t.Run("token เดิมที่ไม่มี iss/aud → 401 (ต้องปล่อย 2 เฟส)", func(t *testing.T) {
		tok := sign(t, key, jwt.MapClaims{"sub": testSub, "exp": time.Now().Add(time.Hour).Unix()})
		if res, _ := strict.Test(bearerReq(tok), -1); res.StatusCode != 401 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})
}

// roles claim + สะพานชั่วคราว ADMIN_USER_IDS
func TestAuthMiddleware_Roles(t *testing.T) {
	key := testKey(t)
	adminID := uuid.MustParse(testSub)

	readRoles := func(app *fiber.App, tok string) []string {
		res, _ := app.Test(bearerReq(tok), -1)
		b, _ := io.ReadAll(res.Body)
		var m struct {
			Roles []string `json:"roles"`
		}
		_ = json.Unmarshal(b, &m)
		return m.Roles
	}

	t.Run("token ที่มี roles → ใช้ตามนั้น", func(t *testing.T) {
		app := authApp(&key.PublicKey)
		tok := sign(t, key, jwt.MapClaims{
			"sub": testSub, "roles": []string{"SUPER_ADMIN"},
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		if got := readRoles(app, tok); len(got) != 1 || got[0] != "SUPER_ADMIN" {
			t.Fatalf("roles = %v", got)
		}
	})

	// สำคัญ: token เดิมที่ออกก่อน Phase 1A ต้องใช้งานได้ต่อในฐานะ USER
	t.Run("token ที่ไม่มี roles → USER (deploy ได้โดยผู้ใช้เดิมไม่พัง)", func(t *testing.T) {
		app := authApp(&key.PublicKey)
		tok := sign(t, key, jwt.MapClaims{"sub": testSub, "exp": time.Now().Add(time.Hour).Unix()})
		if got := readRoles(app, tok); len(got) != 1 || got[0] != domain.RoleUser {
			t.Fatalf("roles = %v ต้องการ [USER]", got)
		}
	})

	t.Run("ADMIN_USER_IDS ให้ SUPER_ADMIN ระหว่างรอ roles claim", func(t *testing.T) {
		app := authApp(&key.PublicKey, func(c *AuthConfig) {
			c.AdminUserIDs = map[uuid.UUID]bool{adminID: true}
		})
		tok := sign(t, key, jwt.MapClaims{"sub": testSub, "exp": time.Now().Add(time.Hour).Unix()})
		got := readRoles(app, tok)
		if len(got) != 1 || got[0] != domain.RoleSuperAdmin {
			t.Fatalf("roles = %v ต้องการ [SUPER_ADMIN]", got)
		}
	})

	t.Run("คนที่ไม่อยู่ใน allowlist ไม่ได้ SUPER_ADMIN", func(t *testing.T) {
		app := authApp(&key.PublicKey, func(c *AuthConfig) {
			c.AdminUserIDs = map[uuid.UUID]bool{uuid.New(): true}
		})
		tok := sign(t, key, jwt.MapClaims{"sub": testSub, "exp": time.Now().Add(time.Hour).Unix()})
		if got := readRoles(app, tok); got[0] == domain.RoleSuperAdmin {
			t.Fatal("ไม่ควรได้ SUPER_ADMIN")
		}
	})
}

// TestKeyRotation ยืนยันว่า rotate key ได้โดย token ที่ยังไม่หมดอายุไม่พัง
//
// นี่คือเงื่อนไขที่ทำให้ rotate key ได้จริงโดยไม่มี downtime:
// ระหว่างเปลี่ยนผ่าน service ต้องยอมรับทั้งใบเก่าและใบใหม่พร้อมกัน
func TestKeyRotation(t *testing.T) {
	oldKey := testKey(t)
	newKey := testKey(t)

	claims := func() jwt.MapClaims {
		return jwt.MapClaims{"sub": testSub, "exp": time.Now().Add(time.Hour).Unix()}
	}

	// ระหว่าง rotate: ยอมรับทั้งสองใบ
	both := authApp(nil, func(c *AuthConfig) {
		c.PublicKeys = []*rsa.PublicKey{&oldKey.PublicKey, &newKey.PublicKey}
	})

	t.Run("token ที่เซ็นด้วยคีย์เก่ายังใช้ได้", func(t *testing.T) {
		if res, _ := both.Test(bearerReq(sign(t, oldKey, claims())), -1); res.StatusCode != 200 {
			t.Fatalf("status = %d — token เก่าต้องยังใช้ได้ระหว่าง rotate", res.StatusCode)
		}
	})

	t.Run("token ที่เซ็นด้วยคีย์ใหม่ใช้ได้", func(t *testing.T) {
		if res, _ := both.Test(bearerReq(sign(t, newKey, claims())), -1); res.StatusCode != 200 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("คีย์ที่ไม่อยู่ในรายการยังถูกปฏิเสธ", func(t *testing.T) {
		rogue := testKey(t)
		if res, _ := both.Test(bearerReq(sign(t, rogue, claims())), -1); res.StatusCode != 401 {
			t.Fatalf("status = %d ต้องการ 401", res.StatusCode)
		}
	})

	// หลัง rotate เสร็จ: เอาใบเก่าออก
	t.Run("เอาคีย์เก่าออกแล้ว token เก่าต้องใช้ไม่ได้", func(t *testing.T) {
		newOnly := authApp(nil, func(c *AuthConfig) {
			c.PublicKeys = []*rsa.PublicKey{&newKey.PublicKey}
		})
		if res, _ := newOnly.Test(bearerReq(sign(t, oldKey, claims())), -1); res.StatusCode != 401 {
			t.Fatalf("status = %d ต้องการ 401", res.StatusCode)
		}
		if res, _ := newOnly.Test(bearerReq(sign(t, newKey, claims())), -1); res.StatusCode != 200 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})
}

// TestKeyID ยืนยันว่า kid คำนวณจากตัวคีย์ได้ค่าเดียวกันเสมอ
// ทำให้ผู้เซ็นและผู้ตรวจไม่ต้องตกลงชื่อ kid กันล่วงหน้า
func TestKeyID(t *testing.T) {
	k := testKey(t)
	id1 := KeyID(&k.PublicKey)
	id2 := KeyID(&k.PublicKey)
	if id1 == "" || id1 != id2 {
		t.Fatalf("kid ต้องคงที่: %q vs %q", id1, id2)
	}
	other := testKey(t)
	if KeyID(&other.PublicKey) == id1 {
		t.Fatal("คีย์คนละใบต้องได้ kid ต่างกัน")
	}
}

// token ที่มี kid ต้องเลือกคีย์ได้ตรงใบ และ kid มั่วต้องถูกปฏิเสธ
func TestKeyID_Routing(t *testing.T) {
	oldKey, newKey := testKey(t), testKey(t)
	app := authApp(nil, func(c *AuthConfig) {
		c.PublicKeys = []*rsa.PublicKey{&oldKey.PublicKey, &newKey.PublicKey}
	})

	withKID := func(k *rsa.PrivateKey, kid string) string {
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"sub": testSub, "exp": time.Now().Add(time.Hour).Unix(),
		})
		tok.Header["kid"] = kid
		s, err := tok.SignedString(k)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	t.Run("kid ตรงกับคีย์ → ผ่าน", func(t *testing.T) {
		tok := withKID(newKey, KeyID(&newKey.PublicKey))
		if res, _ := app.Test(bearerReq(tok), -1); res.StatusCode != 200 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("kid ไม่รู้จัก → 401", func(t *testing.T) {
		tok := withKID(newKey, "kid-ที่ไม่มีอยู่")
		if res, _ := app.Test(bearerReq(tok), -1); res.StatusCode != 401 {
			t.Fatalf("status = %d ต้องการ 401", res.StatusCode)
		}
	})

	// เคสสำคัญ: kid ชี้ไปคีย์ A แต่เซ็นด้วยคีย์ B ต้องไม่ผ่าน
	t.Run("kid ชี้คีย์หนึ่ง แต่เซ็นด้วยอีกคีย์ → 401", func(t *testing.T) {
		tok := withKID(oldKey, KeyID(&newKey.PublicKey))
		if res, _ := app.Test(bearerReq(tok), -1); res.StatusCode != 401 {
			t.Fatalf("status = %d ต้องการ 401", res.StatusCode)
		}
	})
}
