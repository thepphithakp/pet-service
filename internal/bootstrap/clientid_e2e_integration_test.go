//go:build integration

package bootstrap

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vertex/pet-service/internal/config"
	"github.com/vertex/pet-service/pkg/middleware"
)

// TestWaterLog_ClientSuppliedIDIsHonored พิสูจน์บนฐานข้อมูลจริงว่า
// การบันทึกครั้งเดียวได้แถวเดียว และเป็นแถวที่ใช้ id ของ client
//
// อาการที่เจอบน production 2026-08-23: บันทึกน้ำ 10 แล้วแอปแสดงเป็น 20
// เพราะ server ทิ้ง id ที่แอปส่งมาแล้วสร้างใหม่ แอปจึงมีสองรายการ
// (ของตัวเอง + ของ server) และกดลบรายการของตัวเองได้ 404
func TestWaterLog_ClientSuppliedIDIsHonored(t *testing.T) {
	db := openTestDB(t)
	app, key := newClientIDTestApp(t, db)

	petID := seedPet(t, db, clientIDTestOwner)
	clientID := uuid.New()
	t.Cleanup(func() {
		db.Exec("DELETE FROM water_logs WHERE pet_id = ?", petID)
		db.Exec("DELETE FROM pets WHERE id = ?", petID)
	})

	body := fmt.Sprintf(`{"id":%q,"amount":10}`, clientID)
	st, resp := doJSON(t, app, "POST",
		"/api/v1/pets/"+petID.String()+"/water-logs", body, key)
	if st != fiber.StatusCreated {
		t.Fatalf("status = %d ต้องเป็น 201 (%s)", st, resp)
	}

	var created map[string]any
	if err := json.Unmarshal(resp, &created); err != nil {
		t.Fatalf("อ่าน response ไม่ได้: %v", err)
	}
	if created["id"] != clientID.String() {
		t.Errorf("id ที่คืนมา = %v ต้องเป็น %v ที่ client ส่งไป", created["id"], clientID)
	}

	// ฐานข้อมูลต้องมีแถวเดียว
	var n int64
	db.Raw("SELECT count(*) FROM water_logs WHERE pet_id = ?", petID).Scan(&n)
	if n != 1 {
		t.Fatalf("มี %d แถว ต้องมีแถวเดียว", n)
	}

	// GET ต้องคืนแถวเดียวที่ id ตรงกับที่ client สร้าง
	st, listBody := doJSON(t, app, "GET",
		"/api/v1/pets/"+petID.String()+"/water-logs", "", key)
	if st != fiber.StatusOK {
		t.Fatalf("GET status = %d", st)
	}
	var logs []map[string]any
	if err := json.Unmarshal(listBody, &logs); err != nil {
		t.Fatalf("อ่าน list ไม่ได้: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("list มี %d รายการ ต้องมีรายการเดียว — แอปถึงจะไม่แสดงซ้ำ", len(logs))
	}
	if logs[0]["id"] != clientID.String() {
		t.Errorf("id ใน list = %v ต้องเป็น %v", logs[0]["id"], clientID)
	}

	// ลบด้วย id ที่ client ถืออยู่ต้องได้ ไม่ใช่ 404
	st, _ = doJSON(t, app, "DELETE",
		"/api/v1/pets/"+petID.String()+"/water-logs/"+clientID.String(), "", key)
	if st != fiber.StatusNoContent {
		t.Errorf("DELETE status = %d ต้องเป็น 204 — แอปต้องลบรายการของตัวเองได้", st)
	}
}

// TestWaterLog_RepeatedPostIsIdempotent เน็ตหลุดแล้วแอปส่งซ้ำ
// ต้องไม่เกิดแถวที่สอง และต้องไม่เป็น 500
func TestWaterLog_RepeatedPostIsIdempotent(t *testing.T) {
	db := openTestDB(t)
	app, key := newClientIDTestApp(t, db)

	petID := seedPet(t, db, clientIDTestOwner)
	clientID := uuid.New()
	t.Cleanup(func() {
		db.Exec("DELETE FROM water_logs WHERE pet_id = ?", petID)
		db.Exec("DELETE FROM pets WHERE id = ?", petID)
	})

	body := fmt.Sprintf(`{"id":%q,"amount":30}`, clientID)
	path := "/api/v1/pets/" + petID.String() + "/water-logs"

	for i := 1; i <= 3; i++ {
		st, resp := doJSON(t, app, "POST", path, body, key)
		if st >= 500 {
			t.Fatalf("ครั้งที่ %d status = %d ต้องไม่เป็น 5xx (%s)", i, st, resp)
		}
	}

	var n int64
	db.Raw("SELECT count(*) FROM water_logs WHERE pet_id = ?", petID).Scan(&n)
	if n != 1 {
		t.Fatalf("ส่ง 3 ครั้งได้ %d แถว ต้องได้แถวเดียว", n)
	}

	var total int64
	db.Raw("SELECT coalesce(sum(amount),0) FROM water_logs WHERE pet_id = ?", petID).Scan(&total)
	if total != 30 {
		t.Errorf("ยอดรวม = %d ต้องเป็น 30 ไม่ใช่ทวีคูณ", total)
	}
}

// TestWaterLog_IDFromAnotherPetIsRejected
//
// การรับ id จาก client ต้องไม่เปิดช่องให้เขียนทับรายการของสัตว์เลี้ยงตัวอื่น
func TestWaterLog_IDFromAnotherPetIsRejected(t *testing.T) {
	db := openTestDB(t)
	app, key := newClientIDTestApp(t, db)

	petA := seedPet(t, db, clientIDTestOwner)
	petB := seedPet(t, db, clientIDTestOwner)
	sharedID := uuid.New()
	t.Cleanup(func() {
		db.Exec("DELETE FROM water_logs WHERE pet_id IN (?, ?)", petA, petB)
		db.Exec("DELETE FROM pets WHERE id IN (?, ?)", petA, petB)
	})

	body := fmt.Sprintf(`{"id":%q,"amount":10}`, sharedID)
	if st, resp := doJSON(t, app, "POST",
		"/api/v1/pets/"+petA.String()+"/water-logs", body, key); st != fiber.StatusCreated {
		t.Fatalf("บันทึกให้ petA status = %d (%s)", st, resp)
	}

	// ใช้ id เดิมกับสัตว์เลี้ยงอีกตัว
	st, resp := doJSON(t, app, "POST",
		"/api/v1/pets/"+petB.String()+"/water-logs", body, key)
	if st != fiber.StatusConflict {
		t.Errorf("status = %d ต้องเป็น 409 ไม่ใช่ 5xx (%s)", st, resp)
	}

	// ข้อมูลของ petA ต้องไม่ถูกแตะ
	var amount int
	db.Raw("SELECT amount FROM water_logs WHERE id = ?", sharedID).Scan(&amount)
	if amount != 10 {
		t.Errorf("รายการของ petA เปลี่ยนไปเป็น %d", amount)
	}
	var nB int64
	db.Raw("SELECT count(*) FROM water_logs WHERE pet_id = ?", petB).Scan(&nB)
	if nB != 0 {
		t.Errorf("petB มี %d แถว ต้องไม่มีเลย", nB)
	}
}

// --- helper ---

// clientIDTestOwner คือเจ้าของสัตว์เลี้ยงที่เทสต์ชุดนี้ใช้
var clientIDTestOwner = uuid.MustParse("af8ebb2e-1b91-4b12-9aa8-5424a2eb09b9")

func newClientIDTestApp(t *testing.T, db *gorm.DB) (*fiber.App, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("สร้างคีย์ไม่สำเร็จ: %v", err)
	}
	app, _, _, _ := NewApp(db, config.Config{Port: "0"}, middleware.AuthConfig{
		PublicKeys: []*rsa.PublicKey{&key.PublicKey},
	})
	return app, key
}

func doJSON(t *testing.T, app *fiber.App, method, path, body string, key *rsa.PrivateKey) (int, []byte) {
	t.Helper()
	return doJSONAs(t, app, method, path, body, key, clientIDTestOwner)
}

// doJSONAs ยิง request ในนามของ user ที่ระบุ
func doJSONAs(t *testing.T, app *fiber.App, method, path, body string, key *rsa.PrivateKey, user uuid.UUID) (int, []byte) {
	t.Helper()

	var r io.Reader
	if body != "" {
		r = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	signRequest(t, req, key, user)

	resp, err := app.Test(req, 10_000)
	if err != nil {
		t.Fatalf("ยิง request ไม่สำเร็จ: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// signRequest ใส่ Authorization header ที่เซ็นด้วยคีย์ทดสอบ
func signRequest(t *testing.T, req *http.Request, key *rsa.PrivateKey, user uuid.UUID) {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub":   user.String(),
		"roles": []string{"USER"},
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = middleware.KeyID(&key.PublicKey)
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("เซ็น token ไม่สำเร็จ: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+signed)
}
