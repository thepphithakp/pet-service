//go:build integration

package bootstrap

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vertex/pet-service/internal/config"
	"github.com/vertex/pet-service/pkg/middleware"
)

type pageResp struct {
	Data []struct {
		ID   uuid.UUID `json:"id"`
		Date time.Time `json:"date"`
	} `json:"data"`
	NextCursor *string `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
}

func paginationApp(t *testing.T, db *gorm.DB) (*fiber.App, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("สร้างคีย์ไม่สำเร็จ: %v", err)
	}
	app, _, _, _ := NewApp(db, config.Config{Port: "0"},
		middleware.AuthConfig{PublicKeys: []*rsa.PublicKey{&key.PublicKey}})
	return app, key
}

// TestPagination_BackwardCompatible คือข้อกำหนดหลักของงานนี้
//
// ผู้เรียกที่ไม่ส่ง limit หรือ cursor ต้องได้ JSON array แบบเดิมเป๊ะ
// ไม่ใช่ object ที่ห่อไว้ ไม่งั้นแอปที่ใช้อยู่พังทันทีที่ deploy
func TestPagination_BackwardCompatible(t *testing.T) {
	db := openTestDB(t)
	owner := uuid.New()
	app, key := paginationApp(t, db)

	petID := seedPet(t, db, owner)
	t.Cleanup(func() {
		db.Exec("DELETE FROM water_logs WHERE pet_id = ?", petID)
		db.Exec("DELETE FROM pets WHERE id = ?", petID)
	})
	for i := 0; i < 5; i++ {
		insertWater(t, db, petID, time.Now().Add(-time.Duration(i)*time.Hour), (i+1)*10)
	}

	st, body := doJSONAs(t, app, "GET",
		fmt.Sprintf("/api/v1/pets/%s/water-logs", petID), "", key, owner)
	if st != fiber.StatusOK {
		t.Fatalf("status = %d ต้องเป็น 200", st)
	}

	// ต้อง unmarshal เป็น array ได้ตรงๆ
	var arr []map[string]any
	if err := json.Unmarshal(body, &arr); err != nil {
		t.Fatalf("ไม่ส่ง limit มาต้องได้ array แบบเดิม ไม่ใช่ object: %v\n%s", err, body)
	}
	if len(arr) != 5 {
		t.Errorf("ได้ %d รายการ ต้องได้ 5 ครบ", len(arr))
	}
}

// TestPagination_WalksEveryRowExactlyOnce
//
// เดินทีละหน้าจนหมดแล้วต้องได้ทุกแถวครบพอดี ไม่ซ้ำ ไม่ขาด
// จงใจใส่ log ที่วันเดียวกันหลายรายการ เพราะถ้า cursor ใช้แต่ date
// จะวนซ้ำที่เดิมไม่จบ
func TestPagination_WalksEveryRowExactlyOnce(t *testing.T) {
	db := openTestDB(t)
	owner := uuid.New()
	app, key := paginationApp(t, db)

	petID := seedPet(t, db, owner)
	t.Cleanup(func() {
		db.Exec("DELETE FROM water_logs WHERE pet_id = ?", petID)
		db.Exec("DELETE FROM pets WHERE id = ?", petID)
	})

	const total = 17
	sameDay := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	for i := 0; i < total; i++ {
		d := sameDay
		if i%3 == 0 {
			d = sameDay.Add(-time.Duration(i) * time.Hour)
		}
		insertWater(t, db, petID, d, i+1)
	}

	seen := map[uuid.UUID]int{}
	var order []time.Time
	cursor := ""
	for round := 0; round < 20; round++ {
		url := fmt.Sprintf("/api/v1/pets/%s/water-logs?limit=5", petID)
		if cursor != "" {
			url += "&cursor=" + cursor
		}
		st, body := doJSONAs(t, app, "GET", url, "", key, owner)
		if st != fiber.StatusOK {
			t.Fatalf("รอบที่ %d status = %d (%s)", round, st, body)
		}

		var resp pageResp
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("ส่ง limit มาต้องได้ object ที่มี data/nextCursor/hasMore: %v", err)
		}
		if len(resp.Data) > 5 {
			t.Fatalf("ขอ 5 ได้ %d — ไม่เคารพ limit", len(resp.Data))
		}

		for _, row := range resp.Data {
			seen[row.ID]++
			order = append(order, row.Date)
		}

		if !resp.HasMore {
			if resp.NextCursor != nil {
				t.Error("hasMore=false แล้วต้องไม่มี nextCursor")
			}
			break
		}
		if resp.NextCursor == nil {
			t.Fatal("hasMore=true แต่ไม่ให้ nextCursor มา — client เดินต่อไม่ได้")
		}
		cursor = *resp.NextCursor
	}

	if len(seen) != total {
		t.Errorf("เดินครบแล้วเจอ %d แถว ต้องเจอ %d", len(seen), total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("แถว %s ถูกคืนมา %d ครั้ง ต้องครั้งเดียว", id, n)
		}
	}
	for i := 1; i < len(order); i++ {
		if order[i].After(order[i-1]) {
			t.Fatalf("ลำดับข้ามหน้าไม่ต่อเนื่อง ที่ตำแหน่ง %d", i)
		}
	}
}

// TestPagination_NewRowsDoNotShiftPages
//
// เหตุผลที่ใช้ keyset ไม่ใช่ offset — ถ้าใช้ offset การเพิ่ม log ใหม่
// ระหว่างที่ผู้ใช้กำลังเลื่อนดูจะทำให้รายการเลื่อนตำแหน่ง แล้วเห็นซ้ำ
func TestPagination_NewRowsDoNotShiftPages(t *testing.T) {
	db := openTestDB(t)
	owner := uuid.New()
	app, key := paginationApp(t, db)

	petID := seedPet(t, db, owner)
	t.Cleanup(func() {
		db.Exec("DELETE FROM water_logs WHERE pet_id = ?", petID)
		db.Exec("DELETE FROM pets WHERE id = ?", petID)
	})

	base := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 6; i++ {
		insertWater(t, db, petID, base.Add(-time.Duration(i)*time.Hour), i+1)
	}

	url := fmt.Sprintf("/api/v1/pets/%s/water-logs?limit=3", petID)
	st, body := doJSONAs(t, app, "GET", url, "", key, owner)
	if st != fiber.StatusOK {
		t.Fatalf("status = %d", st)
	}
	var first pageResp
	if err := json.Unmarshal(body, &first); err != nil {
		t.Fatalf("อ่าน response ไม่ได้: %v", err)
	}
	if first.NextCursor == nil {
		t.Fatal("ต้องมีหน้าถัดไป")
	}

	// มี log ใหม่เข้ามาระหว่างที่ผู้ใช้กำลังดู
	insertWater(t, db, petID, base.Add(time.Hour), 999)

	st, body = doJSONAs(t, app, "GET", url+"&cursor="+*first.NextCursor, "", key, owner)
	if st != fiber.StatusOK {
		t.Fatalf("status = %d", st)
	}
	var second pageResp
	if err := json.Unmarshal(body, &second); err != nil {
		t.Fatalf("อ่าน response ไม่ได้: %v", err)
	}

	firstIDs := map[uuid.UUID]bool{}
	for _, r := range first.Data {
		firstIDs[r.ID] = true
	}
	for _, r := range second.Data {
		if firstIDs[r.ID] {
			t.Errorf("แถว %s โผล่ซ้ำในหน้าที่สอง — offset pagination จะเป็นแบบนี้", r.ID)
		}
	}
}

// TestPagination_RejectsBadInput ค่าที่ผิดต้องเป็น 400 ไม่ใช่ 500
func TestPagination_RejectsBadInput(t *testing.T) {
	db := openTestDB(t)
	owner := uuid.New()
	app, key := paginationApp(t, db)

	petID := seedPet(t, db, owner)
	t.Cleanup(func() { db.Exec("DELETE FROM pets WHERE id = ?", petID) })

	base := fmt.Sprintf("/api/v1/pets/%s/water-logs", petID)
	for _, tc := range []struct{ name, query string }{
		{"limit ไม่ใช่ตัวเลข", "?limit=มาก"},
		{"limit ติดลบ", "?limit=-1"},
		{"limit เป็นศูนย์", "?limit=0"},
		{"cursor มั่ว", "?cursor=!!!"},
		{"cursor ถอดได้แต่รูปแบบผิด", "?cursor=aGVsbG8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, body := doJSONAs(t, app, "GET", base+tc.query, "", key, owner)
			if st != fiber.StatusBadRequest {
				t.Errorf("status = %d ต้องเป็น 400 (%s)", st, body)
			}
		})
	}
}

// TestPagination_LimitIsCapped ขอเยอะเกินต้องถูกตัด ไม่ใช่ดึงทั้งตาราง
func TestPagination_LimitIsCapped(t *testing.T) {
	db := openTestDB(t)
	owner := uuid.New()
	app, key := paginationApp(t, db)

	petID := seedPet(t, db, owner)
	t.Cleanup(func() {
		db.Exec("DELETE FROM water_logs WHERE pet_id = ?", petID)
		db.Exec("DELETE FROM pets WHERE id = ?", petID)
	})
	for i := 0; i < 3; i++ {
		insertWater(t, db, petID, time.Now().Add(-time.Duration(i)*time.Hour), i+1)
	}

	st, body := doJSONAs(t, app, "GET",
		fmt.Sprintf("/api/v1/pets/%s/water-logs?limit=999999", petID), "", key, owner)
	if st != fiber.StatusOK {
		t.Fatalf("status = %d ต้องเป็น 200 — ขอเกินเพดานให้ตัดให้ ไม่ใช่ปฏิเสธ", st)
	}
	var resp pageResp
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("อ่าน response ไม่ได้: %v", err)
	}
	if len(resp.Data) != 3 {
		t.Errorf("ได้ %d ต้องได้ 3 (ข้อมูลมีแค่นั้น)", len(resp.Data))
	}
}
