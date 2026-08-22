//go:build integration

package bootstrap

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vertex/pet-service/internal/config"
	"github.com/vertex/pet-service/pkg/middleware"
)

// TestAuthorization_E2E พิสูจน์ว่าช่องโหว่ IDOR ปิดแล้วจริง ผ่าน HTTP จริง
// กับฐานข้อมูลจริงที่ผ่าน Flyway migration มาแล้ว
//
// นี่คือ test ที่ตอบคำถาม "แน่ใจได้อย่างไรว่าแก้แล้ว" ได้ตรงที่สุด
func TestAuthorization_E2E(t *testing.T) {
	db := openTestDB(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	owner, stranger, caregiver := uuid.New(), uuid.New(), uuid.New()
	petID := seedPet(t, db, owner)
	t.Cleanup(func() { cleanupPet(db, petID) })

	// caregiver ที่มีเฉพาะ MANAGE_LITTER — ไม่มี EDIT_PROFILE และไม่มี MANAGE_WATER
	cgID := seedCaregiver(t, db, petID, caregiver, "MANAGE_LITTER")
	t.Cleanup(func() { db.Exec(`DELETE FROM pet_caregivers WHERE id = ?`, cgID) })

	app := NewApp(db, config.Config{Port: "0"}, middleware.AuthConfig{PublicKey: &key.PublicKey})

	token := func(uid uuid.UUID, roles ...string) string {
		claims := jwt.MapClaims{
			"sub": uid.String(), "name": "e2e", "exp": time.Now().Add(time.Hour).Unix(),
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

	call := func(method, path, tok, body string) int {
		var rdr *strings.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		var req *http.Request
		if rdr != nil {
			req = httptest.NewRequest(method, path, rdr)
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		res, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		return res.StatusCode
	}

	base := "/api/v1/pets/" + petID.String()

	cases := []struct {
		name   string
		method string
		path   string
		token  string
		body   string
		want   int
	}{
		// --- เจ้าของ ---
		{"เจ้าของ อ่านสัตว์เลี้ยงตัวเอง", "GET", base, token(owner), "", 200},
		{"เจ้าของ แก้ได้", "PUT", base, token(owner), `{"name":"ชื่อใหม่"}`, 200},
		{"เจ้าของ ดู caregiver ได้", "GET", base + "/caregivers", token(owner), "", 200},
		{"เจ้าของ บันทึกน้ำได้", "POST", base + "/water-logs", token(owner), `{"amount":30}`, 201},

		// --- คนนอก: ต้องเป็น 404 ทุกเคส ไม่ใช่ 403 ---
		{"คนนอก อ่าน → 404", "GET", base, token(stranger), "", 404},
		{"คนนอก แก้ → 404", "PUT", base, token(stranger), `{"name":"แฮ็ก"}`, 404},
		{"คนนอก ลบ → 404", "DELETE", base, token(stranger), "", 404},
		{"คนนอก ดู caregiver → 404", "GET", base + "/caregivers", token(stranger), "", 404},
		{"คนนอก อ่าน litter → 404", "GET", base + "/litter-logs", token(stranger), "", 404},
		{"คนนอก เขียน litter → 404", "POST", base + "/litter-logs", token(stranger), `{"type":"Poop","amount":1}`, 404},
		{"คนนอก เขียน water → 404", "POST", base + "/water-logs", token(stranger), `{"amount":30}`, 404},

		// --- caregiver: เห็นได้ แต่สิทธิ์จำกัด → 403 ไม่ใช่ 404 ---
		{"caregiver อ่านได้", "GET", base, token(caregiver), "", 200},
		{"caregiver อ่าน litter ได้", "GET", base + "/litter-logs", token(caregiver), "", 200},
		{"caregiver ที่มี MANAGE_LITTER เขียน litter ได้", "POST", base + "/litter-logs", token(caregiver), `{"type":"Poop","amount":1}`, 201},
		{"caregiver ที่ไม่มี MANAGE_WATER เขียน water ไม่ได้", "POST", base + "/water-logs", token(caregiver), `{"amount":30}`, 403},
		{"caregiver ที่ไม่มี EDIT_PROFILE แก้โปรไฟล์ไม่ได้", "PUT", base, token(caregiver), `{"name":"x"}`, 403},
		{"caregiver ลบสัตว์เลี้ยงไม่ได้", "DELETE", base, token(caregiver), "", 403},
		{"caregiver จัดการ caregiver คนอื่นไม่ได้", "GET", base + "/caregivers", token(caregiver), "", 403},

		// --- admin ---
		{"user ธรรมดา เข้า /admin/pets ไม่ได้", "GET", "/api/v1/admin/pets", token(stranger), "", 403},
		{"SUPER_ADMIN เข้า /admin/pets ได้", "GET", "/api/v1/admin/pets", token(stranger, "SUPER_ADMIN"), "", 200},
		{"SUPER_ADMIN อ่านสัตว์เลี้ยงคนอื่นได้", "GET", base, token(stranger, "SUPER_ADMIN"), "", 200},

		// --- validation ---
		{"amount ติดลบ → 400", "POST", base + "/water-logs", token(owner), `{"amount":-5}`, 400},
		{"ชื่อว่าง → 400", "POST", "/api/v1/pets", token(owner), `{"name":"  "}`, 400},

		// --- ไม่มี token ---
		{"ไม่มี token → 401", "GET", base, "", "", 401},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := call(tc.method, tc.path, tc.token, tc.body); got != tc.want {
				t.Errorf("%s %s → %d ต้องการ %d", tc.method, tc.path, got, tc.want)
			}
		})
	}
}

// TestLogDelete_ScopedToPet ยืนยันว่าลบ log ของสัตว์เลี้ยงตัวอื่นผ่าน path ของตัวเองไม่ได้
func TestLogDelete_ScopedToPet(t *testing.T) {
	db := openTestDB(t)
	key, _ := rsa.GenerateKey(rand.Reader, 2048)

	userA, userB := uuid.New(), uuid.New()
	petA := seedPet(t, db, userA)
	petB := seedPet(t, db, userB)
	t.Cleanup(func() { cleanupPet(db, petA); cleanupPet(db, petB) })

	logB := uuid.New()
	db.Exec(`INSERT INTO litter_logs (id, pet_id, date, type, amount, created_at, updated_at, is_active)
	         VALUES (?, ?, now(), 'Poop', 1, now(), now(), true)`, logB, petB)

	app := NewApp(db, config.Config{Port: "0"}, middleware.AuthConfig{PublicKey: &key.PublicKey})
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": userA.String(), "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(key)

	// userA ใช้ path ของสัตว์เลี้ยงตัวเอง แต่ชี้ไป log ของ petB
	req := httptest.NewRequest("DELETE", "/api/v1/pets/"+petA.String()+"/litter-logs/"+logB.String(), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	res, err := app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 404 {
		t.Fatalf("status = %d ต้องการ 404", res.StatusCode)
	}

	var count int64
	db.Raw(`SELECT count(*) FROM litter_logs WHERE id = ?`, logB).Scan(&count)
	if count != 1 {
		t.Fatal("log ของสัตว์เลี้ยงตัวอื่นถูกลบไปแล้ว — scope ไม่ทำงาน")
	}
}

func seedPet(t *testing.T, db *gorm.DB, owner uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	err := db.Exec(`INSERT INTO pets (id, owner_id, owner_username, name, species, gender,
	                                  is_spayed_neutered, created_at, updated_at)
	                VALUES (?, ?, 'e2e', 'ทดสอบ', 'CAT', 'Female', false, now(), now())`,
		id, owner).Error
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func seedCaregiver(t *testing.T, db *gorm.DB, petID, userID uuid.UUID, perms ...string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := db.Exec(`INSERT INTO pet_caregivers (id, pet_id, user_id, created_at, updated_at)
	                   VALUES (?, ?, ?, now(), now())`, id, petID, userID).Error; err != nil {
		t.Fatal(err)
	}
	for _, p := range perms {
		if err := db.Exec(`INSERT INTO caregiver_permissions (caregiver_model_id, permission_model_id)
		                   VALUES (?, ?)`, id, p).Error; err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func cleanupPet(db *gorm.DB, id uuid.UUID) {
	db.Exec(`DELETE FROM litter_logs WHERE pet_id = ?`, id)
	db.Exec(`DELETE FROM water_logs WHERE pet_id = ?`, id)
	db.Exec(`DELETE FROM pets WHERE id = ?`, id)
}
