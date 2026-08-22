package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// HeaderRequestID คือชื่อ header ที่ใช้ correlate log ข้าม service
const HeaderRequestID = "X-Request-Id"

// NewRequestID รับ request id จาก client ถ้ามี ถ้าไม่มีก็สร้างใหม่
// แล้วเซ็ตทั้งฝั่ง request (ให้ middleware/handler ตัวถัดไปอ่านได้ผ่าน c.Get)
// และฝั่ง response (ให้ client เอาไปอ้างอิงเวลาแจ้งปัญหา)
func NewRequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		reqID := c.Get(HeaderRequestID)
		if reqID == "" {
			reqID = uuid.New().String()
			c.Request().Header.Set(HeaderRequestID, reqID)
		}
		c.Set(HeaderRequestID, reqID)
		return c.Next()
	}
}
