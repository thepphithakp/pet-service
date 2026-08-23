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

// TestLogOrder_IsStableAndNewestFirst
//
// ของเดิม litter ไม่มี ORDER BY เลย (C-11) PostgreSQL จึงคืนลำดับตามใจ
// และลำดับเปลี่ยนได้จริงหลัง UPDATE — รายการในแอปสลับที่เองโดยไม่มีสาเหตุ
//
// เทสต์นี้จงใจใส่ log ที่ "วันเดียวกัน" หลายรายการ เพราะ date อย่างเดียว
// ตัดสินลำดับไม่ได้ ต้องมี tiebreaker ถึงจะนิ่ง
func TestLogOrder_IsStableAndNewestFirst(t *testing.T) {
	db := openTestDB(t)
	owner := uuid.New()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("สร้างคีย์ไม่สำเร็จ: %v", err)
	}
	app, _, _ := NewApp(db, config.Config{Port: "0"},
		middleware.AuthConfig{PublicKeys: []*rsa.PublicKey{&key.PublicKey}})

	petID := seedPet(t, db, owner)
	t.Cleanup(func() {
		db.Exec("DELETE FROM litter_logs WHERE pet_id = ?", petID)
		db.Exec("DELETE FROM water_logs WHERE pet_id = ?", petID)
		db.Exec("DELETE FROM pets WHERE id = ?", petID)
	})

	sameDay := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	older := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		name string
		path string
		seed func()
	}{
		{
			name: "litter",
			path: fmt.Sprintf("/api/v1/pets/%s/litter-logs", petID),
			seed: func() {
				for i, d := range []time.Time{sameDay, older, newer, sameDay, sameDay} {
					insertLitter(t, db, petID, d, i+1)
				}
			},
		},
		{
			name: "water",
			path: fmt.Sprintf("/api/v1/pets/%s/water-logs", petID),
			seed: func() {
				for i, d := range []time.Time{sameDay, older, newer, sameDay, sameDay} {
					insertWater(t, db, petID, d, (i+1)*10)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.seed()

			first := logDates(t, app, tc.path, key, owner)
			if len(first) != 5 {
				t.Fatalf("ได้ %d รายการ ต้องได้ 5", len(first))
			}

			// ใหม่สุดต้องมาก่อน
			for i := 1; i < len(first); i++ {
				if first[i].After(first[i-1]) {
					t.Fatalf("ลำดับไม่ได้เรียงจากใหม่ไปเก่า: %v", first)
				}
			}

			// เรียกซ้ำหลายครั้งต้องได้ลำดับเดิมเป๊ะ
			for i := 0; i < 3; i++ {
				again := logDates(t, app, tc.path, key, owner)
				for j := range first {
					if !first[j].Equal(again[j]) {
						t.Fatalf("เรียกซ้ำครั้งที่ %d ได้ลำดับต่างออกไป", i+1)
					}
				}
			}

			// UPDATE ทำให้ PostgreSQL ย้ายแถวได้ — ลำดับต้องยังเหมือนเดิม
			if err := db.Exec(
				fmt.Sprintf("UPDATE %s_logs SET updated_at = now() WHERE pet_id = ?", tc.name),
				petID).Error; err != nil {
				t.Fatalf("update ไม่สำเร็จ: %v", err)
			}
			afterUpdate := logDates(t, app, tc.path, key, owner)
			for j := range first {
				if !first[j].Equal(afterUpdate[j]) {
					t.Errorf("ลำดับเปลี่ยนหลัง UPDATE — ขาด tiebreaker ที่นิ่งพอ")
					break
				}
			}
		})
	}
}

func insertLitter(t *testing.T, db *gorm.DB, petID uuid.UUID, d time.Time, amount int) {
	t.Helper()
	err := db.Exec(`INSERT INTO litter_logs (id, pet_id, date, type, amount, is_active, created_at, updated_at)
	                VALUES (?, ?, ?, 'Poop', ?, true, now(), now())`,
		uuid.New(), petID, d, amount).Error
	if err != nil {
		t.Fatalf("insert litter ไม่สำเร็จ: %v", err)
	}
}

func insertWater(t *testing.T, db *gorm.DB, petID uuid.UUID, d time.Time, amount int) {
	t.Helper()
	err := db.Exec(`INSERT INTO water_logs (id, pet_id, date, amount, is_active, created_at, updated_at)
	                VALUES (?, ?, ?, ?, true, now(), now())`,
		uuid.New(), petID, d, amount).Error
	if err != nil {
		t.Fatalf("insert water ไม่สำเร็จ: %v", err)
	}
}

func logDates(t *testing.T, app *fiber.App, path string, key *rsa.PrivateKey, user uuid.UUID) []time.Time {
	t.Helper()
	st, body := doJSONAs(t, app, "GET", path, "", key, user)
	if st != fiber.StatusOK {
		t.Fatalf("status = %d ต้องเป็น 200 (%s)", st, body)
	}
	var logs []struct {
		Date time.Time `json:"date"`
	}
	if err := json.Unmarshal(body, &logs); err != nil {
		t.Fatalf("อ่าน response ไม่ได้: %v", err)
	}
	out := make([]time.Time, len(logs))
	for i, l := range logs {
		out[i] = l.Date
	}
	return out
}
