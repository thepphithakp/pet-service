package apperror

import (
	"errors"
	"net/http"

	"github.com/vertex/pet-service/internal/domain"
	"gorm.io/gorm"
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
	case errors.Is(err, domain.ErrValidation):
		return BadRequest(err.Error(), err)
	case errors.Is(err, domain.ErrInvalidPermission):
		return BadRequest(err.Error(), err)
	case errors.Is(err, domain.ErrInvalidID):
		return BadRequest("Invalid ID format", err)
	case errors.Is(err, gorm.ErrRecordNotFound):
		return NotFound("Resource", err)
	default:
		return Internal("An unexpected error occurred", err)
	}
}

// IsAppError checks if err is an *AppError.
func IsAppError(err error, target **AppError) bool {
	if e, ok := err.(*AppError); ok {
		*target = e
		return true
	}
	return false
}
