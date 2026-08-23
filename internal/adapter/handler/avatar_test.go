package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
)

// pngBytes คือ PNG จริงขนาดเล็ก ใช้ทดสอบการเดา content type
var pngBytes = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89,
}

func avatarApp(t *testing.T, uc *fakePetUC) interface {
	Test(*http.Request, ...int) (*http.Response, error)
} {
	t.Helper()
	return petHandlerApp(uc, map[string]any{"userId": uuid.NewString()})
}

// TestGetAvatar_ServesBinaryWithCacheHeaders
func TestGetAvatar_ServesBinaryWithCacheHeaders(t *testing.T) {
	const etag = `"abc123"`
	app := avatarApp(t, &fakePetUC{avatar: &domain.Avatar{Data: pngBytes, ETag: etag}})

	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/pets/"+uuid.NewString()+"/avatar", nil))
	if err != nil {
		t.Fatalf("ยิง request ไม่สำเร็จ: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d ต้องเป็น 200", resp.StatusCode)
	}
	if got := resp.Header.Get("ETag"); got != etag {
		t.Errorf("ETag = %q ต้องเป็น %q", got, etag)
	}
	if cc := resp.Header.Get("Cache-Control"); cc == "" {
		t.Error("ต้องมี Cache-Control ไม่งั้น client โหลดรูปใหม่ทุกครั้ง")
	} else if !bytes.Contains([]byte(cc), []byte("private")) {
		t.Errorf("Cache-Control = %q ต้องเป็น private — รูปเป็นของผู้ใช้แต่ละคน", cc)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q ต้องเดาเป็น image/png ได้", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, pngBytes) {
		t.Error("ข้อมูลรูปที่ส่งกลับไม่ตรงกับต้นฉบับ")
	}
}

// TestGetAvatar_NotModified พิสูจน์ว่า conditional request ประหยัดจริง
func TestGetAvatar_NotModified(t *testing.T) {
	const etag = `"abc123"`
	app := avatarApp(t, &fakePetUC{avatar: &domain.Avatar{Data: pngBytes, ETag: etag}})

	cases := []struct {
		name  string
		match string
		want  int
	}{
		{"ETag ตรง → 304", etag, fiber.StatusNotModified},
		{"weak validator ก็ต้องตรง", "W/" + etag, fiber.StatusNotModified},
		{"หลายค่ามีตัวที่ตรง", `"other", ` + etag, fiber.StatusNotModified},
		{"* ตรงกับทุกอย่าง", "*", fiber.StatusNotModified},
		{"ETag ไม่ตรง → 200", `"different"`, fiber.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/pets/"+uuid.NewString()+"/avatar", nil)
			req.Header.Set("If-None-Match", tc.match)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("ยิง request ไม่สำเร็จ: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d ต้องเป็น %d", resp.StatusCode, tc.want)
			}
			if tc.want == fiber.StatusNotModified {
				body, _ := io.ReadAll(resp.Body)
				if len(body) != 0 {
					t.Errorf("304 ต้องไม่มี body แต่ได้ %d ไบต์", len(body))
				}
			}
		})
	}
}

// TestGetAvatar_NotFound ไม่มีรูปต้องเป็น 404 ไม่ใช่ 200 ที่ body ว่าง
func TestGetAvatar_NotFound(t *testing.T) {
	app := avatarApp(t, &fakePetUC{avatar: nil})

	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/pets/"+uuid.NewString()+"/avatar", nil))
	if err != nil {
		t.Fatalf("ยิง request ไม่สำเร็จ: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("status = %d ต้องเป็น 404", resp.StatusCode)
	}
}

// TestPetList_AvatarFlag คือหัวใจของ Phase 5 ข้อ 1
//
// พิสูจน์ว่าปิด flag แล้ว response ไม่มี avatarData จริง
// และเปิดไว้แล้วยังเหมือนเดิม (แอปที่ใช้อยู่ไม่พัง)
func TestPetList_AvatarFlag(t *testing.T) {
	petID := uuid.New()
	heavy := bytes.Repeat([]byte{0xFF}, 2048)

	t.Run("เปิด flag → ยังมี avatarData เหมือนเดิม", func(t *testing.T) {
		uc := &fakePetUC{allForUser: []domain.Pet{{ID: petID, Name: "แมว", AvatarData: heavy}}}
		app := petHandlerAppWithAvatar(uc, map[string]any{"userId": uuid.NewString()}, true)

		body := getJSON(t, app, "/api/v1/pets")
		if !bytes.Contains(body, []byte("avatarData")) {
			t.Error("เปิด flag ไว้ต้องยังส่ง avatarData — ไม่งั้นแอปที่ใช้อยู่รูปหาย")
		}
	})

	t.Run("ปิด flag → ไม่มี avatarData แต่บอกว่ามีรูป", func(t *testing.T) {
		uc := &fakePetUC{summaries: []domain.PetSummary{{ID: petID, Name: "แมว", HasAvatar: true}}}
		app := petHandlerAppWithAvatar(uc, map[string]any{"userId": uuid.NewString()}, false)

		body := getJSON(t, app, "/api/v1/pets")
		if bytes.Contains(body, []byte("avatarData")) {
			t.Errorf("ปิด flag แล้วต้องไม่มี avatarData: %s", body)
		}

		var items []map[string]any
		if err := json.Unmarshal(body, &items); err != nil {
			t.Fatalf("อ่าน response ไม่ได้: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("ต้องมี 1 รายการ ได้ %d", len(items))
		}
		if items[0]["hasAvatar"] != true {
			t.Error("ต้องบอกว่ามีรูป เพื่อให้ client รู้ว่าควรไปดึงที่ /avatar")
		}
		if items[0]["name"] != "แมว" {
			t.Error("ข้อมูลอื่นต้องยังครบ")
		}
	})
}

func getJSON(t *testing.T, app interface {
	Test(*http.Request, ...int) (*http.Response, error)
}, path string) []byte {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", path, nil))
	if err != nil {
		t.Fatalf("ยิง request ไม่สำเร็จ: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d ต้องเป็น 200", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	return b
}
