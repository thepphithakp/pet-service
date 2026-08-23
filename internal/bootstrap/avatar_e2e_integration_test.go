//go:build integration

package bootstrap

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/config"
	"github.com/vertex/pet-service/pkg/middleware"
)

// bigAvatar จำลองรูปขนาดใกล้เคียงของจริงบน production (ตัวใหญ่สุด 2MB)
func bigAvatar() []byte {
	b := bytes.Repeat([]byte{0x89, 0x50, 0x4E, 0x47}, 128*1024) // 512KB
	copy(b, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	return b
}

func avatarTestApp(t *testing.T, includeAvatar bool) (*fiber.App, *rsa.PrivateKey) {
	t.Helper()
	db := openTestDB(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("สร้างคีย์ไม่สำเร็จ: %v", err)
	}
	app, _, _, _ := NewApp(db, config.Config{Port: "0", PetListIncludeAvatar: includeAvatar},
		middleware.AuthConfig{PublicKeys: []*rsa.PublicKey{&key.PublicKey}})
	return app, key
}

// TestPetList_WithoutAvatarIsDramaticallySmaller คือหลักฐานของ Phase 5 ข้อ 1
//
// บน production ผู้ใช้ที่มีสัตว์เลี้ยง 3 ตัวต้องโหลดเกือบ 4MB ทุกครั้งที่เปิดหน้ารายการ
// เพราะ GET /pets ส่ง avatarData มาด้วย เทสต์นี้วัดขนาดจริงของทั้งสองแบบ
func TestPetList_WithoutAvatarIsDramaticallySmaller(t *testing.T) {
	db := openTestDB(t)
	owner := uuid.New()

	petID := seedPet(t, db, owner)
	t.Cleanup(func() { db.Exec("DELETE FROM pets WHERE id = ?", petID) })
	if err := db.Exec("UPDATE pets SET avatar_data = ? WHERE id = ?", bigAvatar(), petID).Error; err != nil {
		t.Fatalf("ใส่รูปทดสอบไม่สำเร็จ: %v", err)
	}

	sizes := map[bool]int{}
	for _, include := range []bool{true, false} {
		app, key := avatarTestApp(t, include)
		st, body := doJSONAs(t, app, "GET", "/api/v1/pets", "", key, owner)
		if st != fiber.StatusOK {
			t.Fatalf("include=%v status = %d", include, st)
		}
		sizes[include] = len(body)

		if include && !bytes.Contains(body, []byte("avatarData")) {
			t.Error("เปิด flag ต้องยังมี avatarData — แอปที่ใช้อยู่จะได้ไม่พัง")
		}
		if !include {
			if bytes.Contains(body, []byte("avatarData")) {
				t.Error("ปิด flag แล้วต้องไม่มี avatarData")
			}
			var items []map[string]any
			if err := json.Unmarshal(body, &items); err != nil {
				t.Fatalf("อ่าน response ไม่ได้: %v", err)
			}
			if len(items) != 1 || items[0]["hasAvatar"] != true {
				t.Errorf("ต้องบอกว่ามีรูปเพื่อให้ client ไปดึงที่ /avatar: %v", items)
			}
		}
	}

	t.Logf("ขนาด response: มีรูป %d ไบต์ / ไม่มีรูป %d ไบต์", sizes[true], sizes[false])
	if sizes[false]*10 > sizes[true] {
		t.Errorf("ปิด flag แล้วต้องเล็กลงอย่างมีนัยสำคัญ: %d → %d ไบต์", sizes[true], sizes[false])
	}
}

// TestPetList_FlagOnlyChangesAvatar
//
// สวิตช์ PET_LIST_INCLUDE_AVATAR ต้องเปลี่ยนเรื่องรูปอย่างเดียว
// ตอนแรกเขียน query แบบ summary โดยลืม preload caregivers
// ทำให้การปิดสวิตช์จะทำ field นั้นหายไปด้วยโดยไม่ตั้งใจ
func TestPetList_FlagOnlyChangesAvatar(t *testing.T) {
	db := openTestDB(t)
	owner := uuid.New()
	helper := uuid.New()

	petID := seedPet(t, db, owner)
	seedCaregiver(t, db, petID, helper, "MANAGE_WATER")
	t.Cleanup(func() {
		db.Exec("DELETE FROM pet_caregivers WHERE pet_id = ?", petID)
		db.Exec("DELETE FROM pets WHERE id = ?", petID)
	})

	shapes := map[bool]map[string]any{}
	for _, include := range []bool{true, false} {
		app, key := avatarTestApp(t, include)
		st, body := doJSONAs(t, app, "GET", "/api/v1/pets", "", key, owner)
		if st != fiber.StatusOK {
			t.Fatalf("include=%v status = %d", include, st)
		}
		var items []map[string]any
		if err := json.Unmarshal(body, &items); err != nil {
			t.Fatalf("อ่าน response ไม่ได้: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("ต้องมี 1 รายการ ได้ %d", len(items))
		}
		shapes[include] = items[0]
	}

	// field ที่ควรต่างมีแค่เรื่องรูป
	allowedDiff := map[string]bool{"avatarData": true, "hasAvatar": true}
	for k := range shapes[true] {
		if allowedDiff[k] {
			continue
		}
		if _, ok := shapes[false][k]; !ok {
			t.Errorf("ปิดสวิตช์แล้ว field %q หายไป — สวิตช์นี้ควรเปลี่ยนแค่เรื่องรูป", k)
		}
	}

	cg, ok := shapes[false]["caregivers"].([]any)
	if !ok || len(cg) != 1 {
		t.Fatalf("caregivers ต้องยังอยู่ครบ: %v", shapes[false]["caregivers"])
	}
	first, _ := cg[0].(map[string]any)
	if perms, ok := first["permissions"].([]any); !ok || len(perms) == 0 {
		t.Errorf("permissions ของ caregiver ต้องถูก preload มาด้วย: %v", first)
	}
}

// TestAvatarEndpoint_RoundTrip รูปที่ดึงผ่าน endpoint ใหม่ต้องตรงกับต้นฉบับ
// และขอซ้ำด้วย ETag เดิมต้องได้ 304
func TestAvatarEndpoint_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	owner := uuid.New()
	app, key := avatarTestApp(t, false)

	petID := seedPet(t, db, owner)
	t.Cleanup(func() { db.Exec("DELETE FROM pets WHERE id = ?", petID) })
	want := bigAvatar()
	if err := db.Exec("UPDATE pets SET avatar_data = ? WHERE id = ?", want, petID).Error; err != nil {
		t.Fatalf("ใส่รูปทดสอบไม่สำเร็จ: %v", err)
	}

	path := "/api/v1/pets/" + petID.String() + "/avatar"

	req := httptest.NewRequest("GET", path, nil)
	signRequest(t, req, key, owner)
	resp, err := app.Test(req, 10_000)
	if err != nil {
		t.Fatalf("ยิง request ไม่สำเร็จ: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d ต้องเป็น 200", resp.StatusCode)
	}
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, want) {
		t.Fatalf("รูปที่ได้ %d ไบต์ ต้นฉบับ %d ไบต์ — ไม่ตรงกัน", len(got), len(want))
	}

	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("ต้องมี ETag ไม่งั้น client cache ไม่ได้")
	}

	req2 := httptest.NewRequest("GET", path, nil)
	signRequest(t, req2, key, owner)
	req2.Header.Set("If-None-Match", etag)
	resp2, err := app.Test(req2, 10_000)
	if err != nil {
		t.Fatalf("ยิง request ไม่สำเร็จ: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != fiber.StatusNotModified {
		t.Errorf("status = %d ต้องเป็น 304", resp2.StatusCode)
	}
	body2, _ := io.ReadAll(resp2.Body)
	if len(body2) != 0 {
		t.Errorf("304 ต้องไม่มี body แต่ได้ %d ไบต์", len(body2))
	}
}

// TestAvatarEndpoint_RequiresAccess คนอื่นดึงรูปสัตว์เลี้ยงเราไม่ได้
func TestAvatarEndpoint_RequiresAccess(t *testing.T) {
	db := openTestDB(t)
	owner := uuid.New()
	stranger := uuid.New()
	app, key := avatarTestApp(t, false)

	petID := seedPet(t, db, owner)
	t.Cleanup(func() { db.Exec("DELETE FROM pets WHERE id = ?", petID) })
	if err := db.Exec("UPDATE pets SET avatar_data = ? WHERE id = ?", bigAvatar(), petID).Error; err != nil {
		t.Fatalf("ใส่รูปทดสอบไม่สำเร็จ: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/pets/"+petID.String()+"/avatar", nil)
	signRequest(t, req, key, stranger)
	resp, err := app.Test(req, 10_000)
	if err != nil {
		t.Fatalf("ยิง request ไม่สำเร็จ: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == fiber.StatusOK {
		t.Error("คนที่ไม่เกี่ยวข้องต้องดึงรูปไม่ได้")
	}
	body, _ := io.ReadAll(resp.Body)
	if bytes.Contains(body, []byte{0x89, 0x50, 0x4E, 0x47}) {
		t.Error("ข้อมูลรูปรั่วออกไปทั้งที่ไม่มีสิทธิ์")
	}
}
