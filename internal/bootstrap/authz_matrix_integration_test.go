//go:build integration

package bootstrap

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/config"
	"github.com/vertex/pet-service/pkg/middleware"
)

// ผู้เรียกแต่ละแบบที่ระบบต้องแยกออกจากกันให้ได้
type callerKind int

const (
	callerOwner        callerKind = iota // เจ้าของสัตว์เลี้ยง
	callerCaregiverYes                   // ผู้ดูแลที่ได้รับสิทธิ์นั้น
	callerCaregiverNo                    // ผู้ดูแลที่ไม่ได้รับสิทธิ์นั้น
	callerStranger                       // คนนอกที่ไม่เกี่ยวข้องเลย
)

func (c callerKind) String() string {
	switch c {
	case callerOwner:
		return "เจ้าของ"
	case callerCaregiverYes:
		return "ผู้ดูแลที่มีสิทธิ์"
	case callerCaregiverNo:
		return "ผู้ดูแลที่ไม่มีสิทธิ์"
	default:
		return "คนนอก"
	}
}

// TestAuthorizationMatrix ตรวจทุกช่องของ (ผู้เรียก × endpoint)
//
// นี่คือเทสต์ที่กันบั๊กประเภทที่เจ็บที่สุด — คนที่ไม่ควรเห็นข้อมูลกลับเห็นได้
// เขียนเป็นตารางเพื่อให้เพิ่ม endpoint ใหม่แล้วถูกบังคับให้ระบุว่าใครเข้าได้บ้าง
//
// ⚠️ คนนอกต้องได้ 404 ไม่ใช่ 403
//
//	403 เท่ากับยืนยันว่า "สัตว์เลี้ยงตัวนี้มีอยู่จริง" ซึ่งทำให้ไล่เดา UUID
//	เพื่อสำรวจว่าในระบบมีอะไรบ้างได้
func TestAuthorizationMatrix(t *testing.T) {
	db := openTestDB(t)

	owner := uuid.New()
	helper := uuid.New()   // ผู้ดูแลที่ได้สิทธิ์ครบ
	limited := uuid.New()  // ผู้ดูแลที่ได้เฉพาะ MANAGE_WATER
	stranger := uuid.New() // ไม่เกี่ยวข้องเลย

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("สร้างคีย์ไม่สำเร็จ: %v", err)
	}
	app, _, _, _ := NewApp(db, config.Config{Port: "0"},
		middleware.AuthConfig{PublicKeys: []*rsa.PublicKey{&key.PublicKey}})

	petID := seedPet(t, db, owner)
	seedCaregiver(t, db, petID, helper, "MANAGE_WATER", "MANAGE_LITTER", "EDIT_PROFILE")
	seedCaregiver(t, db, petID, limited, "MANAGE_WATER")
	t.Cleanup(func() {
		db.Exec("DELETE FROM event_outbox WHERE entity_id = ?", petID.String())
		db.Exec("DELETE FROM caregiver_permissions WHERE caregiver_model_id IN (SELECT id FROM pet_caregivers WHERE pet_id = ?)", petID)
		db.Exec("DELETE FROM pet_caregivers WHERE pet_id = ?", petID)
		db.Exec("DELETE FROM water_logs WHERE pet_id = ?", petID)
		db.Exec("DELETE FROM litter_logs WHERE pet_id = ?", petID)
		db.Exec("DELETE FROM pets WHERE id = ?", petID)
	})

	userFor := map[callerKind]uuid.UUID{
		callerOwner:        owner,
		callerCaregiverYes: helper,
		callerCaregiverNo:  limited,
		callerStranger:     stranger,
	}

	base := "/api/v1/pets/" + petID.String()

	// want คือ status ที่ต้องได้ของผู้เรียกแต่ละแบบ
	// ok = 2xx ตัวใดตัวหนึ่ง (แล้วแต่ endpoint)
	const ok = -1

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		want   map[callerKind]int
	}{
		{
			name: "อ่านข้อมูลสัตว์เลี้ยง", method: "GET", path: base,
			want: map[callerKind]int{
				callerOwner: ok, callerCaregiverYes: ok, callerCaregiverNo: ok,
				callerStranger: fiber.StatusNotFound,
			},
		},
		{
			name: "อ่านรูป", method: "GET", path: base + "/avatar",
			want: map[callerKind]int{
				// ไม่มีรูป → 404 ทั้งคู่ แต่เหตุผลต่างกัน
				// สิ่งที่ต้องพิสูจน์คือคนนอกต้องไม่ได้ 200 เด็ดขาด
				callerOwner: fiber.StatusNotFound, callerCaregiverYes: fiber.StatusNotFound,
				callerCaregiverNo: fiber.StatusNotFound, callerStranger: fiber.StatusNotFound,
			},
		},
		{
			name: "อ่าน water log", method: "GET", path: base + "/water-logs",
			want: map[callerKind]int{
				callerOwner: ok, callerCaregiverYes: ok, callerCaregiverNo: ok,
				callerStranger: fiber.StatusNotFound,
			},
		},
		{
			name: "บันทึกน้ำ", method: "POST", path: base + "/water-logs",
			body: `{"amount":10}`,
			want: map[callerKind]int{
				callerOwner: ok, callerCaregiverYes: ok,
				// limited มี MANAGE_WATER จึงบันทึกน้ำได้
				callerCaregiverNo: ok,
				callerStranger:    fiber.StatusNotFound,
			},
		},
		{
			name: "บันทึก litter", method: "POST", path: base + "/litter-logs",
			body: `{"type":"Poop","amount":1}`,
			want: map[callerKind]int{
				callerOwner: ok, callerCaregiverYes: ok,
				// limited ไม่มี MANAGE_LITTER → ต้องถูกปฏิเสธ
				callerCaregiverNo: fiber.StatusForbidden,
				callerStranger:    fiber.StatusNotFound,
			},
		},
		{
			name: "แก้ไขข้อมูลสัตว์เลี้ยง", method: "PUT", path: base,
			body: `{"name":"ชื่อใหม่","species":"CAT","breed":"x","colorCode":"#fff","birthDate":"2020-01-01T00:00:00Z","gender":"Female"}`,
			want: map[callerKind]int{
				callerOwner: ok, callerCaregiverYes: ok,
				// limited ไม่มี EDIT_PROFILE
				callerCaregiverNo: fiber.StatusForbidden,
				callerStranger:    fiber.StatusNotFound,
			},
		},
		{
			name: "ลบสัตว์เลี้ยง", method: "DELETE", path: base,
			want: map[callerKind]int{
				// ลบได้เฉพาะเจ้าของ — ผู้ดูแลไม่ว่าจะมีสิทธิ์อะไรก็ลบไม่ได้
				callerOwner:        ok,
				callerCaregiverYes: fiber.StatusForbidden,
				callerCaregiverNo:  fiber.StatusForbidden,
				callerStranger:     fiber.StatusNotFound,
			},
		},
		{
			// รายชื่อผู้ดูแลเป็นข้อมูลของเจ้าของ — ผู้ดูแลด้วยกันไม่ควรเห็นว่า
			// ใครอีกบ้างที่เข้าถึงสัตว์เลี้ยงตัวนี้ได้
			name: "ดูรายชื่อผู้ดูแล", method: "GET", path: base + "/caregivers",
			want: map[callerKind]int{
				callerOwner:        ok,
				callerCaregiverYes: fiber.StatusForbidden,
				callerCaregiverNo:  fiber.StatusForbidden,
				callerStranger:     fiber.StatusNotFound,
			},
		},
	}

	for _, tc := range cases {
		for _, caller := range []callerKind{callerOwner, callerCaregiverYes, callerCaregiverNo, callerStranger} {
			want, defined := tc.want[caller]
			if !defined {
				t.Fatalf("ตารางไม่ครบ: %q ยังไม่ได้ระบุผลของ %s", tc.name, caller)
			}

			// "ลบสัตว์เลี้ยง" ของเจ้าของทำจริงไม่ได้ เพราะจะทำให้เคสที่เหลือพัง
			if tc.method == "DELETE" && caller == callerOwner {
				continue
			}

			t.Run(fmt.Sprintf("%s/%s", tc.name, caller), func(t *testing.T) {
				st, body := doJSONAs(t, app, tc.method, tc.path, tc.body, key, userFor[caller])

				if want == ok {
					if st < 200 || st >= 300 {
						t.Fatalf("status = %d ต้องเป็น 2xx (%s)", st, body)
					}
					return
				}
				if st != want {
					t.Fatalf("status = %d ต้องเป็น %d (%s)", st, want, body)
				}
				// ข้อมูลต้องไม่รั่วออกไปกับ error response
				if caller == callerStranger && len(body) > 0 {
					assertNoPetData(t, body, petID)
				}
			})
		}
	}
}

// assertNoPetData ยืนยันว่า response ไม่มีข้อมูลของสัตว์เลี้ยงหลุดออกไป
func assertNoPetData(t *testing.T, body []byte, petID uuid.UUID) {
	t.Helper()
	for _, leak := range []string{petID.String(), "ทดสอบ", "avatarData"} {
		if containsFold(string(body), leak) {
			t.Errorf("ข้อมูลรั่วใน error response: พบ %q", leak)
		}
	}
}

func containsFold(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		indexFold(haystack, needle) >= 0
}

func indexFold(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if equalFold(s[i:i+len(substr)], substr) {
			return i
		}
	}
	return -1
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// TestAuthorizationMatrix_NoToken ทุก endpoint ต้องปฏิเสธผู้ที่ไม่ได้ยืนยันตัวตน
func TestAuthorizationMatrix_NoToken(t *testing.T) {
	db := openTestDB(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("สร้างคีย์ไม่สำเร็จ: %v", err)
	}
	app, _, _, _ := NewApp(db, config.Config{Port: "0"},
		middleware.AuthConfig{PublicKeys: []*rsa.PublicKey{&key.PublicKey}})

	id := uuid.New().String()
	paths := []struct{ method, path string }{
		{"GET", "/api/v1/pets"},
		{"GET", "/api/v1/pets/" + id},
		{"GET", "/api/v1/pets/" + id + "/avatar"},
		{"GET", "/api/v1/pets/" + id + "/water-logs"},
		{"POST", "/api/v1/pets/" + id + "/water-logs"},
		{"GET", "/api/v1/pets/" + id + "/litter-logs"},
		{"GET", "/api/v1/pets/" + id + "/caregivers"},
		{"GET", "/api/v1/master-data/cat-breeds"},
	}

	for _, p := range paths {
		t.Run(p.method+" "+p.path, func(t *testing.T) {
			st, _ := doUnauthenticated(t, app, p.method, p.path)
			if st != fiber.StatusUnauthorized {
				t.Errorf("status = %d ต้องเป็น 401", st)
			}
		})
	}
}
