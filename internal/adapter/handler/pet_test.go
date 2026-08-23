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
	// true = พฤติกรรมเดิมที่ส่ง avatar ไปกับรายการ ซึ่งเทสต์ชุดเดิมคาดหวังอยู่
	return petHandlerAppWithAvatar(uc, locals, true)
}

func petHandlerAppWithAvatar(uc *fakePetUC, locals map[string]any, includeAvatar bool) interface {
	Test(*http.Request, ...int) (*http.Response, error)
} {
	app := newTestApp(locals)
	h := NewPetHandler(uc, includeAvatar)
	h.RegisterRoutes(app.Group("/api/v1"))
	h.RegisterAdminRoutes(app.Group("/api/v1/admin"))
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

	t.Run("userId ไม่ใช่ uuid → 401 (actor ไม่ถูกเซ็ต)", func(t *testing.T) {
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

	// S-3 แก้แล้ว: createdBy ต้องมาจาก actor เท่านั้น
	//
	// หมายเหตุ: ตอนนี้ยังกันที่ชั้น service (PetService.Create เขียนทับ)
	// Phase 1.3 จะเพิ่ม DTO ที่ไม่มีฟิลด์นี้เลย เพื่อกันตั้งแต่ชั้น handler
	t.Run("S-3: client กำหนด createdBy เองไม่ได้", func(t *testing.T) {
		uc := &fakePetUC{created: &domain.Pet{}}
		app := petHandlerApp(uc, map[string]any{"userId": testUserID})
		do(t, app, "POST", "/api/v1/pets", map[string]any{"name": "x", "createdBy": "hacker"})
		if uc.createdArg.CreatedBy != nil && *uc.createdArg.CreatedBy == "hacker" {
			t.Fatal("createdBy ต้องไม่มาจาก request body")
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

// S-2 แก้แล้ว: การตรวจสิทธิ์อยู่ที่ชั้น service — handler แค่ map error เป็น status
//
// การบังคับจริงมี test ครอบที่ application/pet_service_test.go
// (TestPetService_GetAll_RequiresCapability) ตรงนี้ยืนยันว่า route ยังอยู่
// และ error จาก service ถูกแปลงเป็น 403 ถูกต้อง
func TestPetAdminGetAll_ForbiddenMapsTo403(t *testing.T) {
	uc := &fakePetUC{err: domain.ErrForbidden}
	app := petHandlerApp(uc, map[string]any{"userId": testUserID})
	res := do(t, app, "GET", "/api/v1/admin/pets", nil)
	if res.StatusCode != 403 {
		t.Fatalf("status = %d, ต้องการ 403", res.StatusCode)
	}
}

// ไม่มีสิทธิ์กับ resource ที่ไม่ใช่ของตัวเอง → 404 ไม่ใช่ 403
// เพื่อไม่ให้ client แยกออกว่า UUID ไหนมีอยู่จริง
func TestPetGetOne_NoAccessLooksLikeNotFound(t *testing.T) {
	uc := &fakePetUC{err: domain.ErrPetNotFound}
	app := petHandlerApp(uc, map[string]any{"userId": testUserID})
	res := do(t, app, "GET", "/api/v1/pets/"+uuid.NewString(), nil)
	if res.StatusCode != 404 {
		t.Fatalf("status = %d, ต้องการ 404", res.StatusCode)
	}
}
