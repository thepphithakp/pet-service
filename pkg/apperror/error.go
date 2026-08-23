package apperror

import (
	"errors"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/vertex/pet-service/internal/domain"
)

// AppError is a structured error carrying an HTTP status and user-facing message.
type AppError struct {
	Code    int
	Message string
	Cause   error // internal cause — logged but never sent to client
}

func (e *AppError) Error() string { return e.Message }
func (e *AppError) Unwrap() error { return e.Cause }

// Constructors
func BadRequest(msg string, cause ...error) *AppError {
	e := &AppError{Code: http.StatusBadRequest, Message: msg}
	if len(cause) > 0 {
		e.Cause = cause[0]
	}
	return e
}

func NotFound(resource string, cause ...error) *AppError {
	e := &AppError{Code: http.StatusNotFound, Message: resource + " not found"}
	if len(cause) > 0 {
		e.Cause = cause[0]
	}
	return e
}

func Unauthorized(msg string) *AppError {
	return &AppError{Code: http.StatusUnauthorized, Message: msg}
}

func Internal(msg string, cause error) *AppError {
	return &AppError{Code: http.StatusInternalServerError, Message: msg, Cause: cause}
}

// FromDomain maps domain sentinel errors to AppErrors.
func FromDomain(err error) *AppError {
	switch {
	case errors.Is(err, domain.ErrPetNotFound):
		return NotFound("Pet", err)
	case errors.Is(err, domain.ErrCaregiverNotFound):
		return NotFound("Caregiver", err)
	case errors.Is(err, domain.ErrLitterLogNotFound):
		return NotFound("Litter log", err)
	case errors.Is(err, domain.ErrWaterLogNotFound):
		return NotFound("Water log", err)
	case errors.Is(err, domain.ErrForbidden):
		return &AppError{Code: http.StatusForbidden, Message: "You do not have permission to perform this action", Cause: err}
	case errors.Is(err, domain.ErrUnauthenticated):
		return Unauthorized("Authentication required")
	case errors.Is(err, domain.ErrCaregiverDuplicate):
		return BadRequest("Caregiver already exists for this pet", err)
	case errors.Is(err, domain.ErrMasterDataNotFound):
		return NotFound("Master data", err)
	case errors.Is(err, domain.ErrMasterDataDuplicate):
		return &AppError{Code: http.StatusConflict, Message: "รหัสนี้มีอยู่แล้ว", Cause: err}
	case errors.Is(err, domain.ErrLogIDConflict):
		return &AppError{Code: http.StatusConflict,
			Message: "id ของรายการนี้ถูกใช้ไปแล้วกับสัตว์เลี้ยงตัวอื่น", Cause: err}
	case errors.Is(err, domain.ErrVersionConflict):
		return &AppError{Code: http.StatusConflict,
			Message: "มีคนอื่นแก้ไขรายการนี้ไปแล้ว กรุณาโหลดใหม่แล้วลองอีกครั้ง", Cause: err}
	case errors.Is(err, domain.ErrValidation):
		return BadRequest(err.Error(), err)
	case errors.Is(err, domain.ErrInvalidPermission):
		return BadRequest(err.Error(), err)
	case errors.Is(err, domain.ErrInvalidID):
		return BadRequest("Invalid ID format", err)
	case errors.Is(err, gorm.ErrRecordNotFound):
		return NotFound("Resource", err)
	case isMasterDataViolation(err):
		// ค่าที่ผู้ใช้ส่งมาไม่มีใน master data — เป็นความผิดของ request
		// ไม่ใช่ของระบบ ตอบ 400 พร้อมบอกว่าฟิลด์ไหน แทนที่จะเป็น 500
		// ที่ผู้เรียกไล่สาเหตุไม่ได้เลย
		return BadRequest(masterDataMessage(err), err)
	default:
		return Internal("An unexpected error occurred", err)
	}
}

// fkToField แปลงชื่อ foreign key เป็นชื่อฟิลด์ที่ผู้เรียกรู้จัก
//
// ผูกกับชื่อ constraint ในไฟล์ migration โดยตรง — ถ้าเปลี่ยนชื่อ constraint
// ต้องมาแก้ที่นี่ด้วย เทสต์ใน pkg/apperror ครอบไว้แล้ว
var fkToField = map[string]string{
	"fk_pets_species":     "species",
	"fk_pets_gender":      "gender",
	"fk_litter_logs_type": "type",
}

// isMasterDataViolation บอกว่า error นี้เกิดจากค่าที่ไม่มีใน master data ไหม
//
// ไม่ผูกกับ driver ของ PostgreSQL โดยตรงเพื่อไม่ให้ apperror ต้องรู้จัก pgx
// — เทียบจากข้อความซึ่งมีชื่อ constraint อยู่เสมอ
func isMasterDataViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if !strings.Contains(msg, "foreign key constraint") {
		return false
	}
	for name := range fkToField {
		if strings.Contains(msg, name) {
			return true
		}
	}
	return false
}

func masterDataMessage(err error) string {
	msg := err.Error()
	for name, field := range fkToField {
		if strings.Contains(msg, name) {
			return field + ": ค่าที่ระบุไม่มีอยู่ในรายการที่ใช้ได้"
		}
	}
	return "ค่าที่ระบุไม่มีอยู่ในรายการที่ใช้ได้"
}

// IsAppError checks if err is an *AppError.
//
// C-8: เดิมใช้ type assertion ตรงๆ ทำให้ AppError ที่ถูก wrap ด้วย
// fmt.Errorf("%w") ไม่ถูกจับ แล้วตกไปเป็น 500 แทนที่จะเป็น status ที่ถูกต้อง
func IsAppError(err error, target **AppError) bool {
	return errors.As(err, target)
}
