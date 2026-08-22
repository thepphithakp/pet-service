package middleware

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/vertex/pet-service/pkg/apperror"
)

// ErrorHandler is the global Fiber error handler.
// All handler errors are funneled here — single place for error → response mapping.
func ErrorHandler(c *fiber.Ctx, err error) error {
	reqID := c.Get(HeaderRequestID)

	// 1. Our typed AppError
	var appErr *apperror.AppError
	if apperror.IsAppError(err, &appErr) {
		if appErr.Cause != nil {
			slog.ErrorContext(c.UserContext(), "request ล้มเหลว",
				slog.String("request_id", reqID),
				slog.String("method", c.Method()),
				slog.String("path", c.Path()),
				slog.Int("status", appErr.Code),
				slog.String("message", appErr.Message),
				slog.Any("cause", appErr.Cause),
			)
		}
		return c.Status(appErr.Code).JSON(fiber.Map{
			"error":     appErr.Message,
			"requestId": reqID,
		})
	}

	// 2. Fiber built-in errors (route not found, body limit exceeded, etc.)
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return c.Status(fiberErr.Code).JSON(fiber.Map{
			"error":     fiberErr.Message,
			"requestId": reqID,
		})
	}

	// 3. Untyped / unexpected — log and return generic 500
	slog.ErrorContext(c.UserContext(), "error ที่ไม่ได้จัดประเภทไว้",
		slog.String("request_id", reqID),
		slog.String("method", c.Method()),
		slog.String("path", c.Path()),
		slog.Int("status", fiber.StatusInternalServerError),
		slog.Any("error", err),
	)
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"error":     "Internal server error",
		"requestId": reqID,
	})
}
