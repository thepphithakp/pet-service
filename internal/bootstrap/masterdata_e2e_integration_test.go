//go:build integration

package bootstrap

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/config"
	"github.com/vertex/pet-service/pkg/middleware"
)

type mdEnv struct {
	app        *fiber.App
	adminToken string
	userToken  string
}

func newMasterDataEnv(t *testing.T) *mdEnv {
	t.Helper()
	db := openTestDB(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	app, _, _ := NewApp(db, config.Config{Port: "0"}, middleware.AuthConfig{
		PublicKeys: []*rsa.PublicKey{&key.PublicKey},
	})

	mk := func(roles ...string) string {
		claims := jwt.MapClaims{
			"sub": uuid.NewString(), "name": "e2e", "exp": time.Now().Add(time.Hour).Unix(),
		}
		if len(roles) > 0 {
			claims["roles"] = roles
		}
		s, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	return &mdEnv{app: app, adminToken: mk("SUPER_ADMIN"), userToken: mk()}
}

func (e *mdEnv) call(t *testing.T, method, path, token, body string) (int, []byte) {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := e.app.Test(req, 5000)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

// TestMasterDataV1_GoldenValues คือ gate ที่สำคัญที่สุดของ Phase 3
//
// master data ย้ายจาก hardcode ในโค้ดมาอยู่ในฐานข้อมูลแล้ว
// API v1 ต้องคืนค่าเหมือนเดิมทุกตัวอักษร ไม่งั้น client ที่ใช้อยู่พัง
func TestMasterDataV1_GoldenValues(t *testing.T) {
	env := newMasterDataEnv(t)

	status, body := env.call(t, "GET", "/api/v1/master-data/cat-breeds", env.userToken, "")
	if status != 200 {
		t.Fatalf("status = %d: %s", status, body)
	}

	var got []string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("v1 ต้องคืน array ของ string ธรรมดา ไม่ใช่ object: %s", body)
	}

	want := []string{
		"Scottish Fold (หูพับ)", "Scottish Straight (หูตั้ง)", "British Shorthair",
		"Persian", "Maine Coon", "Siamese (วิเชียรมาศ)", "Khao Manee (ขาวมณี)",
		"Sphynx", "Bengal", "Ragdoll", "American Shorthair",
		"Exotic Shorthair", "Munchkin (ขาสั้น)", "Mixed / Other (พันธุ์ผสม/อื่นๆ)",
	}
	if len(got) != len(want) {
		t.Fatalf("ได้ %d รายการ ต้องการ %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q ต้องการ %q", i, got[i], want[i])
		}
	}

	status, body = env.call(t, "GET", "/api/v1/master-data/blood-types", env.userToken, "")
	var blood []string
	_ = json.Unmarshal(body, &blood)
	if status != 200 || len(blood) != 4 || blood[0] != "Unknown" {
		t.Fatalf("blood-types = %s", body)
	}
}

// C-13: endpoint ใหม่ที่หน้าตั้งสิทธิ์ของ backoffice ต้องใช้
func TestMasterData_Permissions(t *testing.T) {
	env := newMasterDataEnv(t)
	status, body := env.call(t, "GET", "/api/v1/master-data/pet-permissions", env.userToken, "")
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	var perms []map[string]any
	_ = json.Unmarshal(body, &perms)
	if len(perms) != 6 {
		t.Fatalf("ได้ %d permission ต้องการ 6 (รวม MANAGE_WATER)", len(perms))
	}
}

func TestMasterDataV2_Structured(t *testing.T) {
	env := newMasterDataEnv(t)
	status, body := env.call(t, "GET", "/api/v1/v2/master-data/cat-breeds", env.userToken, "")
	if status != 200 {
		t.Fatalf("status = %d: %s", status, body)
	}
	var items []map[string]any
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 || items[0]["code"] != "SCOTTISH_FOLD" {
		t.Fatalf("v2 = %s", body)
	}
	// label ต้องเป็นค่าเดียวกับที่ v1 คืน เพื่อให้ client ย้ายมาทีละขั้นได้
	if items[0]["label"] != "Scottish Fold (หูพับ)" {
		t.Fatalf("label = %v", items[0]["label"])
	}
}

// TestMasterDataAdmin_FullLifecycle ครอบวงจรที่ backoffice ใช้จริง
func TestMasterDataAdmin_FullLifecycle(t *testing.T) {
	env := newMasterDataEnv(t)
	code := "TEST_" + strings.ToUpper(uuid.NewString()[:8])
	base := "/api/v1/admin/master-data/cat-breeds"

	db := openTestDB(t)
	t.Cleanup(func() { db.Exec(`DELETE FROM mst_cat_breeds WHERE code = ?`, code) })

	t.Run("user ธรรมดาเพิ่มไม่ได้", func(t *testing.T) {
		status, _ := env.call(t, "POST", base, env.userToken,
			`{"code":"`+code+`","nameEn":"Test","speciesCode":"CAT","sortOrder":500}`)
		if status != 403 {
			t.Fatalf("status = %d ต้องการ 403", status)
		}
	})

	var version int
	t.Run("admin เพิ่มได้", func(t *testing.T) {
		status, body := env.call(t, "POST", base, env.adminToken,
			`{"code":"`+code+`","nameEn":"สายพันธุ์ทดสอบ","nameTh":"ทดสอบ","speciesCode":"CAT","sortOrder":500}`)
		if status != 201 {
			t.Fatalf("status = %d: %s", status, body)
		}
		var item map[string]any
		_ = json.Unmarshal(body, &item)
		version = int(item["version"].(float64))
		if version != 1 {
			t.Fatalf("version = %d ต้องการ 1", version)
		}
	})

	t.Run("รหัสซ้ำ → 409", func(t *testing.T) {
		status, _ := env.call(t, "POST", base, env.adminToken,
			`{"code":"`+code+`","nameEn":"ซ้ำ","speciesCode":"CAT"}`)
		if status != 409 {
			t.Fatalf("status = %d ต้องการ 409", status)
		}
	})

	t.Run("ค่าใหม่โผล่ใน v1 ทันที", func(t *testing.T) {
		// cache TTL 30 วินาที แต่ admin เพิ่งแก้ผ่าน replica เดียวกัน cache ถูกล้างแล้ว
		status, body := env.call(t, "GET", "/api/v1/master-data/cat-breeds", env.userToken, "")
		if status != 200 || !strings.Contains(string(body), "สายพันธุ์ทดสอบ") {
			t.Fatalf("ค่าที่เพิ่มใหม่ต้องโผล่ทันที: %s", body)
		}
	})

	t.Run("แก้ไขด้วย version ที่ถูกต้อง", func(t *testing.T) {
		status, body := env.call(t, "PUT", base+"/"+code, env.adminToken,
			`{"nameEn":"แก้แล้ว","speciesCode":"CAT","sortOrder":501,"version":`+itoa(version)+`}`)
		if status != 200 {
			t.Fatalf("status = %d: %s", status, body)
		}
		var item map[string]any
		_ = json.Unmarshal(body, &item)
		if int(item["version"].(float64)) != version+1 {
			t.Fatalf("version ต้องเพิ่มขึ้น: %v", item["version"])
		}
	})

	// optimistic locking: ส่ง version เก่ามาต้องถูกปฏิเสธ
	t.Run("version เก่า → 409", func(t *testing.T) {
		status, _ := env.call(t, "PUT", base+"/"+code, env.adminToken,
			`{"nameEn":"แก้ทับ","speciesCode":"CAT","version":`+itoa(version)+`}`)
		if status != 409 {
			t.Fatalf("status = %d ต้องการ 409", status)
		}
	})

	t.Run("ไม่ส่ง version → 400", func(t *testing.T) {
		status, _ := env.call(t, "PUT", base+"/"+code, env.adminToken,
			`{"nameEn":"ไม่มี version","speciesCode":"CAT"}`)
		if status != 400 {
			t.Fatalf("status = %d ต้องการ 400", status)
		}
	})

	t.Run("ดูจำนวนที่ใช้อยู่", func(t *testing.T) {
		status, body := env.call(t, "GET", base+"/"+code+"/usage", env.adminToken, "")
		if status != 200 || !strings.Contains(string(body), "usageCount") {
			t.Fatalf("status = %d: %s", status, body)
		}
	})

	t.Run("DELETE เป็นการปิดการใช้งาน ไม่ใช่ลบ", func(t *testing.T) {
		status, body := env.call(t, "DELETE", base+"/"+code, env.adminToken, "")
		if status != 200 {
			t.Fatalf("status = %d: %s", status, body)
		}
		var n int64
		db.Raw(`SELECT count(*) FROM mst_cat_breeds WHERE code = ?`, code).Scan(&n)
		if n != 1 {
			t.Fatal("แถวต้องยังอยู่ในฐานข้อมูล ไม่ถูกลบทิ้ง")
		}
		var active bool
		db.Raw(`SELECT is_active FROM mst_cat_breeds WHERE code = ?`, code).Scan(&active)
		if active {
			t.Fatal("is_active ต้องเป็น false")
		}
	})

	t.Run("ค่าที่ปิดแล้วหายจาก v1", func(t *testing.T) {
		_, body := env.call(t, "GET", "/api/v1/master-data/cat-breeds", env.userToken, "")
		if strings.Contains(string(body), "แก้แล้ว") {
			t.Fatal("ค่าที่ปิดการใช้งานแล้วต้องไม่โผล่ใน dropdown")
		}
	})
}

// TestNewLitterTypeUsableImmediately พิสูจน์ว่าถอด CHECK constraint ออกถูกแล้ว
//
// admin เพิ่มชนิดใหม่ผ่าน backoffice → ต้องบันทึก log ด้วยชนิดนั้นได้ทันที
// โดยไม่ต้อง deploy และไม่ต้องรัน migration
func TestNewLitterTypeUsableImmediately(t *testing.T) {
	env := newMasterDataEnv(t)
	db := openTestDB(t)

	code := "TEST" + uuid.NewString()[:6]
	t.Cleanup(func() {
		db.Exec(`DELETE FROM litter_logs WHERE type = ?`, code)
		db.Exec(`DELETE FROM mst_litter_types WHERE code = ?`, code)
	})

	status, body := env.call(t, "POST", "/api/v1/admin/master-data/litter-types", env.adminToken,
		`{"code":"`+code+`","nameEn":"ชนิดใหม่","sortOrder":900}`)
	if status != 201 {
		t.Fatalf("เพิ่มชนิดใหม่: status = %d: %s", status, body)
	}

	owner := uuid.New()
	petID := seedPet(t, db, owner)
	t.Cleanup(func() { cleanupPet(db, petID) })

	// ใช้ token ของเจ้าของสัตว์เลี้ยงตัวนั้น
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	app, _, _ := NewApp(db, config.Config{Port: "0"}, middleware.AuthConfig{
		PublicKeys: []*rsa.PublicKey{&key.PublicKey},
	})
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": owner.String(), "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(key)

	req := httptest.NewRequest("POST", "/api/v1/pets/"+petID.String()+"/litter-logs",
		strings.NewReader(`{"type":"`+code+`","amount":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	res, err := app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 201 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("บันทึก log ด้วยชนิดใหม่: status = %d: %s\n"+
			"ถ้าเป็น 500 แปลว่ายังมี CHECK constraint ค้างอยู่", res.StatusCode, b)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
