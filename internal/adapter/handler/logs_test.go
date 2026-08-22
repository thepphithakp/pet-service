package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
)

type testApp interface {
	Test(*http.Request, ...int) (*http.Response, error)
}

func litterApp(uc *fakeLitterUC, locals map[string]any) testApp {
	app := newTestApp(locals)
	NewLitterHandler(uc).RegisterRoutes(app.Group("/api/v1"))
	return app
}

func waterApp(uc *fakeWaterUC, locals map[string]any) testApp {
	app := newTestApp(locals)
	NewWaterHandler(uc).RegisterRoutes(app.Group("/api/v1"))
	return app
}

func TestLitterRoutes(t *testing.T) {
	petID := uuid.New()
	base := "/api/v1/pets/" + petID.String() + "/litter-logs"

	t.Run("GET คืน 200", func(t *testing.T) {
		uc := &fakeLitterUC{list: []domain.LitterLog{{ID: uuid.New(), Type: "Poop", Amount: 1}}}
		res := do(t, litterApp(uc, nil), "GET", base, nil)
		if res.StatusCode != 200 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("GET ที่ error → 500 Failed to fetch litter logs", func(t *testing.T) {
		res := do(t, litterApp(&fakeLitterUC{err: errBoom}, nil), "GET", base, nil)
		if res.StatusCode != 500 {
			t.Fatalf("status = %d", res.StatusCode)
		}
		if got := decode(t, res)["error"]; got != "Failed to fetch litter logs" {
			t.Fatalf("error = %v", got)
		}
	})

	t.Run("POST เซ็ต petId จาก path และ createdBy จาก token", func(t *testing.T) {
		uc := &fakeLitterUC{one: &domain.LitterLog{}}
		app := litterApp(uc, map[string]any{"userId": testUserID, "userName": "เทพ"})
		res := do(t, app, "POST", base, map[string]any{"type": "Poop", "amount": 1})
		if res.StatusCode != 201 {
			t.Fatalf("status = %d, ต้องการ 201", res.StatusCode)
		}
		if uc.createArg.PetID != petID {
			t.Fatalf("petID = %s, ต้องมาจาก path", uc.createArg.PetID)
		}
		if uc.createArg.CreatedBy == nil || *uc.createArg.CreatedBy != testUserID {
			t.Fatal("createdBy ต้องมาจาก token")
		}
		if uc.createArg.CreatedByUsername == nil || *uc.createArg.CreatedByUsername != "เทพ" {
			t.Fatal("createdByUsername ต้องมาจาก token")
		}
	})

	// พฤติกรรมปัจจุบัน: ไม่มี validation เลย — type อะไรก็ผ่าน, amount ติดลบก็ผ่าน
	t.Run("known gap: ไม่มี validation ของ type และ amount", func(t *testing.T) {
		uc := &fakeLitterUC{one: &domain.LitterLog{}}
		app := litterApp(uc, map[string]any{"userId": testUserID})
		res := do(t, app, "POST", base, map[string]any{"type": "ค่ามั่ว", "amount": -5})
		if res.StatusCode == 201 {
			t.Log("ยืนยันว่ายังไม่มี validation — Phase 1.3 ต้องทำให้เป็น 400")
			return
		}
		t.Log("validation ถูกเพิ่มแล้ว — อัปเดต test ให้ยืนยัน 400")
	})

	t.Run("POST batch คืน 201 และเซ็ต petId ทุกแถว", func(t *testing.T) {
		uc := &fakeLitterUC{batch: []domain.LitterLog{{}, {}}}
		app := litterApp(uc, map[string]any{"userId": testUserID})
		res := do(t, app, "POST", base+"/batch", []map[string]any{
			{"type": "Poop", "amount": 1}, {"type": "Pee", "amount": 2},
		})
		if res.StatusCode != 201 {
			t.Fatalf("status = %d", res.StatusCode)
		}
		if len(uc.batchArg) != 2 {
			t.Fatalf("batchArg len = %d", len(uc.batchArg))
		}
		for i, l := range uc.batchArg {
			if l.PetID != petID {
				t.Fatalf("แถว %d petID ไม่ตรง", i)
			}
		}
	})

	t.Run("DELETE คืน 204", func(t *testing.T) {
		res := do(t, litterApp(&fakeLitterUC{}, nil), "DELETE", base+"/"+uuid.NewString(), nil)
		if res.StatusCode != 204 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("logId ไม่ใช่ uuid → 400", func(t *testing.T) {
		res := do(t, litterApp(&fakeLitterUC{}, nil), "DELETE", base+"/abc", nil)
		if res.StatusCode != 400 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})
}

func TestWaterRoutes(t *testing.T) {
	petID := uuid.New()
	base := "/api/v1/pets/" + petID.String() + "/water-logs"

	t.Run("GET คืน 200", func(t *testing.T) {
		uc := &fakeWaterUC{list: []domain.WaterLog{{ID: uuid.New(), Amount: 30}}}
		res := do(t, waterApp(uc, nil), "GET", base, nil)
		if res.StatusCode != 200 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("GET ที่ error → 500 Failed to retrieve water logs", func(t *testing.T) {
		res := do(t, waterApp(&fakeWaterUC{err: errBoom}, nil), "GET", base, nil)
		if got := decode(t, res)["error"]; got != "Failed to retrieve water logs" {
			t.Fatalf("error = %v", got)
		}
	})

	t.Run("POST คืน 201 และเซ็ต petId + createdBy", func(t *testing.T) {
		uc := &fakeWaterUC{one: &domain.WaterLog{}}
		app := waterApp(uc, map[string]any{"userId": testUserID, "userName": "เทพ"})
		res := do(t, app, "POST", base, map[string]any{"amount": 30})
		if res.StatusCode != 201 {
			t.Fatalf("status = %d", res.StatusCode)
		}
		if uc.createArg.PetID != petID {
			t.Fatal("petID ต้องมาจาก path")
		}
	})

	t.Run("DELETE คืน 204", func(t *testing.T) {
		res := do(t, waterApp(&fakeWaterUC{}, nil), "DELETE", base+"/"+uuid.NewString(), nil)
		if res.StatusCode != 204 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})
}

func TestCaregiverRoutes(t *testing.T) {
	petID := uuid.New()
	base := "/api/v1/pets/" + petID.String() + "/caregivers"

	newApp := func(uc *fakeCaregiverUC) testApp {
		app := newTestApp(nil)
		NewCaregiverHandler(uc).RegisterRoutes(app.Group("/api/v1"))
		return app
	}

	t.Run("GET คืน 200", func(t *testing.T) {
		uc := &fakeCaregiverUC{list: []domain.PetCaregiver{{ID: uuid.New()}}}
		if res := do(t, newApp(uc), "GET", base, nil); res.StatusCode != 200 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("POST userId ไม่ใช่ uuid → 400", func(t *testing.T) {
		res := do(t, newApp(&fakeCaregiverUC{}), "POST", base, map[string]any{"userId": "abc"})
		if res.StatusCode != 400 {
			t.Fatalf("status = %d", res.StatusCode)
		}
		if got := decode(t, res)["error"]; got != "Invalid user ID" {
			t.Fatalf("error = %v", got)
		}
	})

	t.Run("POST สำเร็จ → 201", func(t *testing.T) {
		uc := &fakeCaregiverUC{one: &domain.PetCaregiver{ID: uuid.New()}}
		res := do(t, newApp(uc), "POST", base, map[string]any{"userId": uuid.NewString()})
		if res.StatusCode != 201 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	// BUG S-4: รับ permission object เต็มก้อนจาก client → GORM upsert ตาราง master
	t.Run("known bug S-4: client ส่ง permission object เต็มก้อนได้", func(t *testing.T) {
		uc := &fakeCaregiverUC{one: &domain.PetCaregiver{}}
		cgID := uuid.NewString()
		do(t, newApp(uc), "PUT", base+"/"+cgID, map[string]any{
			"permissions": []map[string]any{
				{"id": "EDIT_PROFILE", "name": "ชื่อที่ถูกแก้โดย client", "isActive": true},
			},
		})
		if len(uc.permArg) == 1 && uc.permArg[0].Name == "ชื่อที่ถูกแก้โดย client" {
			t.Log("ยืนยันช่องโหว่ S-4 — Phase 1.4 ต้องเปลี่ยนเป็นรับแค่ []string ของ ID")
			return
		}
		t.Log("S-4 ถูกแก้แล้ว")
	})

	t.Run("DELETE คืน 204", func(t *testing.T) {
		res := do(t, newApp(&fakeCaregiverUC{}), "DELETE", base+"/"+uuid.NewString(), nil)
		if res.StatusCode != 204 {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})
}

// TestMasterDataResponseShape ล็อก response shape เดิมไว้เป็น golden
// Phase 3.6 บอกว่า v1 ต้องคืนค่าเหมือนเดิมทุกตัวอักษรหลังย้าย master data เข้า DB
func TestMasterDataResponseShape(t *testing.T) {
	app := newTestApp(nil)
	NewMasterDataHandler(masterDataStub{}).RegisterRoutes(app.Group("/api/v1"))

	for _, tc := range []struct{ path, want string }{
		{"/api/v1/master-data/cat-breeds", `["Scottish Fold (หูพับ)","Persian"]`},
		{"/api/v1/master-data/blood-types", `["Unknown","A"]`},
	} {
		res := do(t, app, "GET", tc.path, nil)
		if res.StatusCode != 200 {
			t.Fatalf("%s status = %d", tc.path, res.StatusCode)
		}
		b, _ := io.ReadAll(res.Body)
		var got []string
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("%s ต้องเป็น array ของ string ธรรมดา ไม่ใช่ object: %s", tc.path, b)
		}
		var want []string
		_ = json.Unmarshal([]byte(tc.want), &want)
		if len(got) != len(want) {
			t.Fatalf("%s: got %v want %v", tc.path, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%s[%d]: got %q want %q", tc.path, i, got[i], want[i])
			}
		}
	}
}
