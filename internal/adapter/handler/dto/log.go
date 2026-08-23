package dto

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
)

// maxLogAmount กันค่าที่เป็นไปไม่ได้ ไม่ได้อิงหน่วยใดหน่วยหนึ่ง
// litter นับเป็นครั้ง water เป็นมิลลิลิตร
const maxLogAmount = 10000

// LitterLogRequest — ไม่มี petId, createdBy, createdByUsername, isActive
// petId มาจาก path เสมอ ส่วน createdBy มาจาก token
//
// ⚠️ ID รับจาก client โดยตั้งใจ
//
// แอปสร้าง UUID เองแล้วแสดงรายการทันทีก่อนที่ POST จะกลับมา (optimistic update)
// ถ้า server ไม่ใช้ id ที่ส่งมาแล้วสร้างใหม่ พอ refresh จะได้อีกแถวที่ id คนละตัว
// แอปจึงแสดงสองรายการจากการบันทึกครั้งเดียว และลบรายการที่แอปสร้างเองได้ 404
// (เกิดขึ้นจริงกับ water log เมื่อ 2026-08-23)
//
// การให้ client กำหนด id ยังทำให้ POST ซ้ำเป็น idempotent ซึ่งจำเป็นกับ
// การใช้งานแบบ offline sync — เป็นแพตเทิร์นเดียวกับที่ BatchLitterLogRequest ใช้อยู่
//
// ปลอดภัยเพราะ id เป็นแค่ตัวระบุแถว ไม่ได้ให้สิทธิ์อะไร
// ส่วนฟิลด์ที่ให้สิทธิ์ (createdBy) ยังมาจาก token เท่านั้น
type LitterLogRequest struct {
	ID     uuid.UUID `json:"id"`
	Date   time.Time `json:"date"`
	Type   string    `json:"type"`
	Amount int       `json:"amount"`
}

func (r LitterLogRequest) Validate() error {
	var errs []string
	if strings.TrimSpace(r.Type) == "" {
		errs = append(errs, "type: ต้องระบุ")
	}
	if len([]rune(r.Type)) > 50 {
		errs = append(errs, "type: ยาวเกิน 50 ตัวอักษร")
	}
	if r.Amount <= 0 || r.Amount > maxLogAmount {
		errs = append(errs, fmt.Sprintf("amount: ต้องอยู่ระหว่าง 1 ถึง %d", maxLogAmount))
	}
	if !r.Date.IsZero() && r.Date.After(time.Now().Add(24*time.Hour)) {
		errs = append(errs, "date: ต้องไม่อยู่ในอนาคต")
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %s", domain.ErrValidation, strings.Join(errs, ", "))
	}
	return nil
}

func (r LitterLogRequest) ToDomain() domain.LitterLog {
	date := r.Date
	if date.IsZero() {
		date = time.Now()
	}
	return domain.LitterLog{ID: r.ID, Date: date, Type: r.Type, Amount: r.Amount, IsActive: true}
}

// BatchLitterLogRequest เหมือน LitterLogRequest ทุกอย่าง
//
// เดิมแยกไว้เพราะมีแต่ batch ที่รับ id ตอนนี้ single ก็รับแล้ว
// คงชื่อไว้เพื่อไม่ให้ต้องแก้ handler ที่เรียกใช้อยู่
type BatchLitterLogRequest struct {
	LitterLogRequest
}

// WaterLogRequest — ไม่มี petId, createdBy, isActive
//
// ⚠️ ID รับจาก client ด้วยเหตุผลเดียวกับ LitterLogRequest ข้างบน
type WaterLogRequest struct {
	ID     uuid.UUID `json:"id"`
	Date   time.Time `json:"date"`
	Amount int       `json:"amount"`
}

func (r WaterLogRequest) Validate() error {
	var errs []string
	if r.Amount <= 0 || r.Amount > maxLogAmount {
		errs = append(errs, fmt.Sprintf("amount: ต้องอยู่ระหว่าง 1 ถึง %d", maxLogAmount))
	}
	if !r.Date.IsZero() && r.Date.After(time.Now().Add(24*time.Hour)) {
		errs = append(errs, "date: ต้องไม่อยู่ในอนาคต")
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %s", domain.ErrValidation, strings.Join(errs, ", "))
	}
	return nil
}

func (r WaterLogRequest) ToDomain() domain.WaterLog {
	date := r.Date
	if date.IsZero() {
		date = time.Now()
	}
	return domain.WaterLog{ID: r.ID, Date: date, Amount: r.Amount, IsActive: true}
}

// AddCaregiverRequest
type AddCaregiverRequest struct {
	UserID string `json:"userId"`
}

// UpdatePermissionsRequest รับแค่ ID (S-4)
type UpdatePermissionsRequest struct {
	PermissionIDs []string `json:"permissionIds"`
	// Permissions รองรับ payload รูปแบบเดิมชั่วคราว อ่านเฉพาะ id เท่านั้น
	Permissions []struct {
		ID string `json:"id"`
	} `json:"permissions"`
}

// IDs รวม payload ทั้งสองรูปแบบให้เป็นรายการเดียว
func (r UpdatePermissionsRequest) IDs() []string {
	if len(r.PermissionIDs) > 0 {
		return r.PermissionIDs
	}
	ids := make([]string, 0, len(r.Permissions))
	for _, p := range r.Permissions {
		ids = append(ids, p.ID)
	}
	return ids
}
