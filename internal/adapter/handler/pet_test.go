package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
)

// decode อ่าน body เป็น map เพื่อตรวจ shape ของ response
func decode(t *testing.T, r *http.Response) map[string]any {
	t.Helper()
	b, _ := io.ReadAll(r.Body)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("body ไม่ใช่ JSON object: %s", b)
	}
	return m
}

func do(t *testing.T, app interface {
	Test(*http.Request, ...int) (*http.Response, error)
}, method, path string, body any) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return res
}

const testUserID = "11111111-1111-1111-1111-111111111111"

func petHandlerApp(uc *fakePetUC, locals map[string]any) interface {
	Test(*http.Request, ...int) (*http.Response, error)
} {
	app := newTestApp(locals)
	NewPetHandler(uc).RegisterRoutes(app.Group("/api/v1"))
	return app
}

func TestPetGetAll(t *testing.T) {
	t.Run("คืน 200 พร้อม array", func(t *testing.T) {
		uc := &fakePetUC{allForUser: []domain.Pet{{ID: uuid.New(), Name: "มะลิ"}}}
		res := do(t, petHandlerApp(uc, map[string]any{"userId": testUserID}), "GET", "/api/v1/pets", nil)
		if res.StatusCode != 200 {
			t.Fatalf("status = %d, ต้องการ 200", res.StatusCode)
		}
		b, _ := io.ReadAll(res.Body)
		var pets []map[string]any
		if err := json.Unmarshal(b, &pets); err != nil {
			t.Fatalf("response ต้องเป็น array: %s", b)
		}
		if len(pets) != 1 || pets[0]["name"] != "มะลิ" {
			t.Fatalf("response ไม่ตรง: %s", b)
		}
	})

	t.Run("ไม่มี userId ใน locals → 401", func(t *testing.T) {
		res := do(t, petHandlerApp(&fakePetUC{}, nil), "GET", "/api/v1/pets", nil)
		if res.StatusCode != 401 {
			t.Fatalf("status = %d, ต้องการ 401", res.StatusCode)
		}
		if got := decode(t, res)["error"]; got != "Missing user ID in token" {
			t.Fatalf("error = %v", got)
		}
	})

	t.Run("userId ไม่ใช่ uuid → 401", func(t *testing.T) {
		res := do(t, petHandlerApp(&fakePetUC{}, map[string]any{"userId": "not-a-uuid"}), "GET", "/api/v1/pets", nil)
		if res.StatusCode != 401 {
			t.Fatalf("status = %d, ต้องการ 401", res.StatusCode)
		}
	})
}

func TestPetGetOne(t *testing.T) {
	t.Run("id ไม่ใช่ uuid → 400", func(t *testing.T) {
		res := do(t, petHandlerApp(&fakePetUC{}, nil), "GET", "/api/v1/pets/abc", nil)
		if res.StatusCode != 400 {
			t.Fatalf("status = %d, ต้องการ 400", res.StatusCode)
		}
		if got := decode(t, res)["error"]; got != "Invalid pet ID" {
			t.Fatalf("error = %v", got)
		}
	})

	t.Run("ไม่พบ → 404 พร้อมข้อความ Pet not found", func(t *testing.T) {
		uc := &fakePetUC{err: domain.ErrPetNotFound}
		res := do(t, petHandlerApp(uc, nil), "GET", "/api/v1/pets/"+uuid.NewString(), nil)
		if res.StatusCode != 404 {
			t.Fatalf("status = %d, ต้องการ 404", res.StatusCode)
		}
		if got := decode(t, res)["error"]; got != "Pet not found" {
			t.Fatalf("error = %v", got)
		}
	})

	t.Run("error ที่ไม่รู้จัก → 500 และไม่รั่วรายละเอียดภายใน", func(t *testing.T) {
		uc := &fakePetUC{err: errBoom}
		res := do(t, petHandlerApp(uc, nil), "GET", "/api/v1/pets/"+uuid.NewString(), nil)
		if res.StatusCode != 500 {
			t.Fatalf("status = %d, ต้องการ 500", res.StatusCode)
		}
		body := decode(t, res)
		if body["error"] != "An unexpected error occurred" {
			t.Fatalf("error = %v", body["error"])
		}
		if _, hasReqID := body["requestId"]; !hasReqID {
			t.Fatal("response ต้องมี requestId")
		}
	})
}

func TestPetCreate(t *testing.T) {
	t.Run("สำเร็จ → 201", func(t *testing.T) {
		id := uuid.New()
		uc := &fakePetUC{created: &domain.Pet{ID: id, Name: "ส้ม"}}
		app := petHandlerApp(uc, map[string]any{"userId": testUserID, "userName": "เทพ"})
		res := do(t, app, "POST", "/api/v1/pets", map[string]any{"name": "ส้ม", "species": "Cat"})
		if res.StatusCode != 201 {
			t.Fatalf("status = %d, ต้องการ 201", res.StatusCode)
		}
		if uc.ownerArg.String() != testUserID {
			t.Fatalf("ownerID = %s, ต้องการ %s", uc.ownerArg, testUserID)
		}
		// handler เซ็ต OwnerUsername จาก token ทับค่าที่ client ส่งมา
		if uc.createdArg.OwnerUsername != "เทพ" {
			t.Fatalf("OwnerUsername = %q, ต้องมาจาก token", uc.createdArg.OwnerUsername)
		}
	})

	t.Run("body ไม่ใช่ JSON → 400", func(t *testing.T) {
		app := petHandlerApp(&fakePetUC{}, map[string]any{"userId": testUserID})
		req := httptest.NewRequest("POST", "/api/v1/pets", bytes.NewReader([]byte("{{{")))
		req.Header.Set("Content-Type", "application/json")
		res, _ := app.Test(req, -1)
		if res.StatusCode != 400 {
			t.Fatalf("status = %d, ต้องการ 400", res.StatusCode)
		}
	})

	// BUG S-3 (mass assignment): client ส่ง createdBy มาเองได้ เพราะ handler bind domain.Pet ตรงๆ
	// test นี้ล็อกพฤติกรรม "ที่ผิด" ไว้ เพื่อให้ Phase 1.3 มาแก้แล้วเห็นชัดว่าเปลี่ยนอะไร
	t.Run("known bug S-3: client กำหนด createdBy ได้", func(t *testing.T) {
		uc := &fakePetUC{created: &domain.Pet{}}
		app := petHandlerApp(uc, map[string]any{"userId": testUserID})
		do(t, app, "POST", "/api/v1/pets", map[string]any{"name": "x", "createdBy": "hacker"})
		if uc.createdArg.CreatedBy == nil || *uc.createdArg.CreatedBy != "hacker" {
			t.Skip("S-3 ถูกแก้แล้ว — อัปเดต test นี้ให้ยืนยันว่า createdBy ต้องไม่มาจาก body")
		}
	})
}

func TestPetDelete(t *testing.T) {
	uc := &fakePetUC{}
	id := uuid.New()
	res := do(t, petHandlerApp(uc, nil), "DELETE", "/api/v1/pets/"+id.String(), nil)
	if res.StatusCode != 204 {
		t.Fatalf("status = %d, ต้องการ 204", res.StatusCode)
	}
	if uc.deletedID != id {
		t.Fatalf("deletedID = %s, ต้องการ %s", uc.deletedID, id)
	}
}

// BUG S-2: /admin/pets ไม่เช็ค role — token ธรรมดาก็ดึงข้อมูลทั้งระบบได้
func TestPetAdminGetAll_KnownVulnerability(t *testing.T) {
	uc := &fakePetUC{all: []domain.Pet{{Name: "ของคนอื่น"}}}
	app := petHandlerApp(uc, map[string]any{"userId": testUserID})
	res := do(t, app, "GET", "/api/v1/admin/pets", nil)
	if res.StatusCode == 200 {
		t.Log("ยืนยันช่องโหว่ S-2: user ธรรมดาเข้า /admin/pets ได้ — Phase 1.2 ต้องทำให้เป็น 403")
		return
	}
	t.Log("S-2 ถูกแก้แล้ว — เปลี่ยน test นี้เป็นการยืนยัน 403")
}
