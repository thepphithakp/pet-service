package dto

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vertex/pet-service/internal/domain"
)

func TestCreatePetRequest_Validate(t *testing.T) {
	valid := CreatePetRequest{Name: "มะลิ", Species: "CAT"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("ควรผ่าน: %v", err)
	}

	cases := []struct {
		name string
		req  CreatePetRequest
	}{
		{"ชื่อว่าง", CreatePetRequest{Name: "   "}},
		{"ชื่อยาวเกิน", CreatePetRequest{Name: strings.Repeat("ก", 101)}},
		{"วันเกิดอยู่ในอนาคต", CreatePetRequest{Name: "x", BirthDate: time.Now().Add(48 * time.Hour)}},
		{"avatar ใหญ่เกิน", CreatePetRequest{Name: "x", AvatarData: make([]byte, maxAvatarSize+1)}},
		{"น้ำหนักติดลบ", CreatePetRequest{Name: "x", CurrentWeight: ptr(-1.0)}},
		{"น้ำหนักเกินจริง", CreatePetRequest{Name: "x", CurrentWeight: ptr(999.0)}},
		{"ข้อความยาวเกิน", CreatePetRequest{Name: "x", Allergies: ptr(strings.Repeat("ก", maxTextLen+1))}},
		{"species ยาวเกิน", CreatePetRequest{Name: "x", Species: strings.Repeat("a", 101)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("ต้องได้ ErrValidation ได้ %v", err)
			}
		})
	}
}

// S-3: DTO ต้องไม่มีฟิลด์ที่ client ไม่ควรกำหนดเอง
func TestCreatePetRequest_NoPrivilegedFields(t *testing.T) {
	// ถ้ามีคนเผลอเพิ่มฟิลด์พวกนี้กลับเข้ามา test จะไม่ compile หรือ ToDomain จะหลุด
	got := CreatePetRequest{Name: "x"}.ToDomain()
	if got.ID.String() != "00000000-0000-0000-0000-000000000000" {
		t.Fatal("DTO ต้องไม่กำหนด ID")
	}
	if got.OwnerID.String() != "00000000-0000-0000-0000-000000000000" {
		t.Fatal("DTO ต้องไม่กำหนด OwnerID")
	}
	if got.CreatedBy != nil || got.UpdatedBy != nil {
		t.Fatal("DTO ต้องไม่กำหนด CreatedBy/UpdatedBy")
	}
	if got.Caregivers != nil {
		t.Fatal("DTO ต้องไม่กำหนด Caregivers")
	}
	if got.OwnerUsername != "" {
		t.Fatal("DTO ต้องไม่กำหนด OwnerUsername")
	}
}

func TestLitterLogRequest_Validate(t *testing.T) {
	if err := (LitterLogRequest{Type: "Poop", Amount: 1}).Validate(); err != nil {
		t.Fatalf("ควรผ่าน: %v", err)
	}
	for _, tc := range []struct {
		name string
		req  LitterLogRequest
	}{
		{"type ว่าง", LitterLogRequest{Amount: 1}},
		{"amount เป็น 0", LitterLogRequest{Type: "Poop", Amount: 0}},
		{"amount ติดลบ", LitterLogRequest{Type: "Poop", Amount: -5}},
		{"amount เกินจริง", LitterLogRequest{Type: "Poop", Amount: 99999}},
		{"date อยู่ในอนาคตไกล", LitterLogRequest{Type: "Poop", Amount: 1, Date: time.Now().Add(72 * time.Hour)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.req.Validate(); !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("ต้องได้ ErrValidation ได้ %v", err)
			}
		})
	}
}

// type ที่ไม่ใช่ Poop/Pee ต้องผ่าน validation ชั้นนี้
// เพราะ admin เพิ่มชนิดใหม่ผ่าน backoffice ได้ — hardcode enum ไว้จะทำให้ใช้ไม่ได้
func TestLitterLogRequest_NoHardcodedEnum(t *testing.T) {
	req := LitterLogRequest{Type: "VOMIT", Amount: 1}
	if err := req.Validate(); err != nil {
		t.Fatalf("ชนิดใหม่ที่ admin เพิ่มผ่าน UI ต้องผ่าน validation ชั้นนี้: %v", err)
	}
}

func TestWaterLogRequest_Validate(t *testing.T) {
	if err := (WaterLogRequest{Amount: 30}).Validate(); err != nil {
		t.Fatalf("ควรผ่าน: %v", err)
	}
	if err := (WaterLogRequest{Amount: 0}).Validate(); !errors.Is(err, domain.ErrValidation) {
		t.Fatal("amount 0 ต้องไม่ผ่าน")
	}
}

func TestLogRequest_DefaultsDateToNow(t *testing.T) {
	l := LitterLogRequest{Type: "Poop", Amount: 1}.ToDomain()
	if l.Date.IsZero() {
		t.Fatal("ไม่ส่ง date มา ต้อง default เป็นเวลาปัจจุบัน")
	}
	w := WaterLogRequest{Amount: 30}.ToDomain()
	if w.Date.IsZero() {
		t.Fatal("ไม่ส่ง date มา ต้อง default เป็นเวลาปัจจุบัน")
	}
}

func TestUpdatePermissionsRequest_IDs(t *testing.T) {
	// รูปแบบใหม่
	r := UpdatePermissionsRequest{PermissionIDs: []string{"A", "B"}}
	if got := r.IDs(); len(got) != 2 {
		t.Fatalf("IDs = %v", got)
	}
	// รูปแบบเดิม — อ่านเฉพาะ id
	r2 := UpdatePermissionsRequest{Permissions: []struct {
		ID string `json:"id"`
	}{{ID: "A"}}}
	if got := r2.IDs(); len(got) != 1 || got[0] != "A" {
		t.Fatalf("IDs = %v", got)
	}
}

func ptr[T any](v T) *T { return &v }
