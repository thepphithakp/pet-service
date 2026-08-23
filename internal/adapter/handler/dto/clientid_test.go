package dto

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// TestLogRequest_KeepsClientSuppliedID กันไม่ให้ regression เดิมกลับมา
//
// แอป iOS สร้าง UUID เองแล้วแสดงรายการทันทีก่อน POST จะกลับมา
// (optimistic update) ถ้า DTO ไม่มีฟิลด์ id ค่าที่แอปส่งมาจะถูกทิ้ง
// server สร้าง id ใหม่ พอแอป refresh จะได้อีกแถวที่ id คนละตัว
// เลยแสดงสองรายการจากการบันทึกครั้งเดียว (บันทึก 10 เห็นเป็น 20)
// และกดลบรายการที่แอปสร้างเองจะได้ 404 เพราะ id นั้นไม่เคยมีในฐานข้อมูล
//
// เกิดขึ้นจริงบน production 2026-08-23
func TestLogRequest_KeepsClientSuppliedID(t *testing.T) {
	clientID := uuid.MustParse("1b17cbe6-32c2-4261-97e5-546ec2a723f1")

	t.Run("water", func(t *testing.T) {
		var r WaterLogRequest
		body := `{"id":"1B17CBE6-32C2-4261-97E5-546EC2A723F1","amount":10}`
		if err := json.Unmarshal([]byte(body), &r); err != nil {
			t.Fatalf("อ่าน body ไม่ได้: %v", err)
		}
		if r.ID != clientID {
			t.Fatalf("ID = %v ต้องเป็น %v", r.ID, clientID)
		}
		if got := r.ToDomain(); got.ID != clientID {
			t.Errorf("ToDomain ทิ้ง id ที่ client ส่งมา: %v", got.ID)
		}
	})

	t.Run("litter", func(t *testing.T) {
		var r LitterLogRequest
		body := `{"id":"1B17CBE6-32C2-4261-97E5-546EC2A723F1","type":"clumping","amount":1}`
		if err := json.Unmarshal([]byte(body), &r); err != nil {
			t.Fatalf("อ่าน body ไม่ได้: %v", err)
		}
		if got := r.ToDomain(); got.ID != clientID {
			t.Errorf("ToDomain ทิ้ง id ที่ client ส่งมา: %v", got.ID)
		}
	})

	t.Run("batch litter ยังทำงานเหมือนเดิม", func(t *testing.T) {
		var rs []BatchLitterLogRequest
		body := `[{"id":"1B17CBE6-32C2-4261-97E5-546EC2A723F1","type":"clumping","amount":1}]`
		if err := json.Unmarshal([]byte(body), &rs); err != nil {
			t.Fatalf("อ่าน body ไม่ได้: %v", err)
		}
		if got := rs[0].ToDomain(); got.ID != clientID {
			t.Errorf("ToDomain ทิ้ง id ที่ client ส่งมา: %v", got.ID)
		}
	})
}

// TestLogRequest_NoIDStillWorks client เก่าที่ไม่ส่ง id ต้องใช้งานได้เหมือนเดิม
//
// service จะเติม uuid ให้เองเมื่อเป็น uuid.Nil
func TestLogRequest_NoIDStillWorks(t *testing.T) {
	var r WaterLogRequest
	if err := json.Unmarshal([]byte(`{"amount":10}`), &r); err != nil {
		t.Fatalf("อ่าน body ไม่ได้: %v", err)
	}
	if got := r.ToDomain(); got.ID != uuid.Nil {
		t.Errorf("ไม่ส่ง id มาต้องได้ uuid.Nil เพื่อให้ service เติมให้: %v", got.ID)
	}
}

// TestLogRequest_IgnoresPrivilegedFields
//
// การรับ id กลับมาต้องไม่เปิดช่องให้ client ตั้งค่าฟิลด์ที่ให้สิทธิ์
func TestLogRequest_IgnoresPrivilegedFields(t *testing.T) {
	body := `{
		"id":"1B17CBE6-32C2-4261-97E5-546EC2A723F1",
		"amount":10,
		"petId":"00000000-0000-0000-0000-000000000099",
		"createdBy":"someone-else",
		"createdByUsername":"attacker",
		"isActive":false
	}`

	var r WaterLogRequest
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("อ่าน body ไม่ได้: %v", err)
	}
	got := r.ToDomain()

	if got.PetID != uuid.Nil {
		t.Error("petId ต้องมาจาก path เท่านั้น")
	}
	if got.CreatedBy != nil || got.CreatedByUsername != nil {
		t.Error("createdBy ต้องมาจาก token เท่านั้น")
	}
	if !got.IsActive {
		t.Error("isActive ต้องถูกกำหนดโดย server ไม่ใช่ client")
	}
}
