package apperror

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/vertex/pet-service/internal/domain"
)

func TestFromDomain(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
		msg  string
	}{
		{"pet not found", domain.ErrPetNotFound, http.StatusNotFound, "Pet not found"},
		{"caregiver not found", domain.ErrCaregiverNotFound, http.StatusNotFound, "Caregiver not found"},
		{"litter not found", domain.ErrLitterLogNotFound, http.StatusNotFound, "Litter log not found"},
		{"duplicate", domain.ErrCaregiverDuplicate, http.StatusBadRequest, "Caregiver already exists for this pet"},
		{"invalid id", domain.ErrInvalidID, http.StatusBadRequest, "Invalid ID format"},
		{"gorm not found", gorm.ErrRecordNotFound, http.StatusNotFound, "Resource not found"},
		{"unknown", errors.New("x"), http.StatusInternalServerError, "An unexpected error occurred"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FromDomain(tc.err)
			if got.Code != tc.code || got.Message != tc.msg {
				t.Fatalf("got %d/%q want %d/%q", got.Code, got.Message, tc.code, tc.msg)
			}
		})
	}
}

// FromDomain ต้องทำงานกับ error ที่ถูก wrap ด้วย %w ได้ (ใช้ errors.Is อยู่แล้ว)
func TestFromDomain_Wrapped(t *testing.T) {
	wrapped := fmt.Errorf("repo layer: %w", domain.ErrPetNotFound)
	if got := FromDomain(wrapped); got.Code != http.StatusNotFound {
		t.Fatalf("wrapped sentinel ต้องได้ 404, ได้ %d", got.Code)
	}
}

// BUG C-8: IsAppError ใช้ type assertion ตรงๆ ไม่ unwrap
// → AppError ที่ถูก wrap จะไม่ถูกจับ กลายเป็น 500 ที่ ErrorHandler
func TestIsAppError_DoesNotUnwrap_KnownBug(t *testing.T) {
	var target *AppError
	direct := BadRequest("ตรงๆ")
	if !IsAppError(direct, &target) {
		t.Fatal("AppError ตรงๆ ต้องถูกจับได้")
	}

	wrapped := fmt.Errorf("ห่อไว้: %w", BadRequest("ข้างใน"))
	var t2 *AppError
	if IsAppError(wrapped, &t2) {
		t.Log("C-8 ถูกแก้แล้ว (ใช้ errors.As) — ลบ t.Log นี้ออกได้")
		return
	}
	t.Log("ยืนยัน bug C-8: AppError ที่ถูก wrap ไม่ถูกจับ → กลายเป็น 500 — Phase 4.1 ต้องเปลี่ยนไปใช้ errors.As")
}

func TestAppError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	e := Internal("พัง", cause)
	if !errors.Is(e, cause) {
		t.Fatal("AppError ต้อง unwrap ไปถึง cause ได้")
	}
	if e.Error() != "พัง" {
		t.Fatalf("Error() = %q", e.Error())
	}
}

// TestFromDomain_MasterDataViolationIsBadRequest
//
// ค่าที่ผู้ใช้ส่งมาไม่มีใน master data เคยกลายเป็น 500 ซึ่งผู้เรียกไล่สาเหตุไม่ได้
// และดูเหมือนระบบพัง ทั้งที่เป็นความผิดของ request
//
// เจอตอนเขียน authorization matrix — ส่ง species ที่ไม่มีจริงแล้วได้ 500
func TestFromDomain_MasterDataViolationIsBadRequest(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantField string
	}{
		{
			"species ไม่มีในรายการ",
			errors.New(`ERROR: insert or update on table "pets" violates foreign key constraint "fk_pets_species" (SQLSTATE 23503)`),
			"species",
		},
		{
			"gender ไม่มีในรายการ",
			errors.New(`ERROR: insert or update on table "pets" violates foreign key constraint "fk_pets_gender" (SQLSTATE 23503)`),
			"gender",
		},
		{
			"litter type ไม่มีในรายการ",
			errors.New(`ERROR: insert or update on table "litter_logs" violates foreign key constraint "fk_litter_logs_type" (SQLSTATE 23503)`),
			"type",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FromDomain(tc.err)
			if got.Code != http.StatusBadRequest {
				t.Errorf("status = %d ต้องเป็น 400", got.Code)
			}
			if !strings.Contains(got.Message, tc.wantField) {
				t.Errorf("ข้อความ %q ต้องบอกว่าฟิลด์ %q ผิด", got.Message, tc.wantField)
			}
		})
	}
}

// TestFromDomain_OtherFKViolationStaysInternal
//
// FK ที่ไม่ได้มาจากค่าที่ผู้ใช้เลือก (เช่น pet_id ที่ระบบใส่เอง)
// ยังต้องเป็น 500 เพราะเป็นความผิดพลาดของระบบจริงๆ
func TestFromDomain_OtherFKViolationStaysInternal(t *testing.T) {
	err := errors.New(`ERROR: violates foreign key constraint "fk_litter_logs_pet" (SQLSTATE 23503)`)
	if got := FromDomain(err); got.Code != http.StatusInternalServerError {
		t.Errorf("status = %d ต้องเป็น 500", got.Code)
	}
}
