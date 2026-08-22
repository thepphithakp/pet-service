package middleware

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/gofiber/fiber/v2"
)

// maskRegex ซ่อนค่าของ field ที่ไม่ควรลงไปอยู่ใน log
// หมายเหตุ: regex ตัวเดียวครอบได้จำกัด — Phase 1.6 จะเปลี่ยนไปใช้ denylist รวมศูนย์
var maskRegex = regexp.MustCompile(`"(AvatarData|avatarData|avatar_data|token)":\s*"[^"]*"`)

func maskJSON(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return maskRegex.ReplaceAllString(string(data), `"$1": "[HIDDEN]"`)
}

// NewAccessLog เขียน access log เป็น JSON หนึ่งบรรทัดต่อหนึ่ง request
//
// ⚠️ พฤติกรรมปัจจุบัน: log ทั้ง request body และ response body ทุก request
// ซึ่งอันตรายเมื่อรวมกับ BodyLimit 50MB และการอัปโหลด avatar (S-8)
// Phase 1.6 จะเปลี่ยนเป็น: default ไม่ log body, ใช้ log/slog, และจำกัดขนาด
func NewAccessLog() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		reqID := c.Get(HeaderRequestID)

		err := c.Next()

		latency := time.Since(start)
		reqBody := maskJSON(c.Body())
		resBody := maskJSON(c.Response().Body())

		logEntry := map[string]interface{}{
			"time":          time.Now().Format("2006-01-02T15:04:05.999Z07:00"),
			"level":         "info",
			"method":        c.Method(),
			"path":          c.Path(),
			"status":        c.Response().StatusCode(),
			"latency":       latency.String(),
			"request_id":    reqID,
			"user_id":       c.Locals("userId"),
			"source_system": c.Get("X-Source-System"),
			"device_id":     c.Get("X-Device-Id"),
			"ip":            c.IP(),
		}

		if body, ok := parseBody(reqBody); ok {
			logEntry["req_body"] = body
		}
		if body, ok := parseBody(resBody); ok {
			logEntry["res_body"] = body
		}

		logJSON, _ := json.Marshal(logEntry)
		fmt.Println(string(logJSON))

		return err
	}
}

// parseBody พยายาม parse เป็น JSON เพื่อให้ log อ่านง่าย ถ้าไม่ได้ก็เก็บเป็นสตริง
func parseBody(raw string) (interface{}, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var obj interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err == nil {
		return obj, true
	}
	return raw, true
}
