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

// LitterLogRequest — ไม่มี id, petId, createdBy, createdByUsername, isActive
// petId มาจาก path เสมอ ส่วน createdBy มาจาก token
type LitterLogRequest struct {
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
	return domain.LitterLog{Date: date, Type: r.Type, Amount: r.Amount, IsActive: true}
}

// BatchLitterLogRequest ยังรับ id ได้
//
// การให้ client กำหนด id เองเป็นแพตเทิร์น idempotency ของ offline sync
// endpoint นี้ถูกเพิ่มมาเพื่อการนั้น จึงคงไว้เพื่อไม่ให้ client ที่ใช้อยู่พัง
type BatchLitterLogRequest struct {
	ID uuid.UUID `json:"id"`
	LitterLogRequest
}

func (r BatchLitterLogRequest) ToDomain() domain.LitterLog {
	l := r.LitterLogRequest.ToDomain()
	l.ID = r.ID
	return l
}

// WaterLogRequest — ไม่มี id, petId, createdBy, isActive
type WaterLogRequest struct {
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
	return domain.WaterLog{Date: date, Amount: r.Amount, IsActive: true}
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
