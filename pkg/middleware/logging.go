package middleware

import (
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// LogConfig ควบคุมความละเอียดของ access log
type LogConfig struct {
	// LogBody เปิดการ log request/response body
	//
	// ⚠️ default = false โดยตั้งใจ
	// ของเดิม log body ทุก request เสมอ ซึ่งอันตรายมากเมื่อรวมกับ
	// BodyLimit 50MB และการอัปโหลด avatar — log บวมจนกลืน disk
	// และข้อมูลส่วนตัวของผู้ใช้ไหลเข้าไปอยู่ในระบบ log (S-8)
	//
	// เปิดได้เฉพาะ non-production เพื่อ debug เท่านั้น
	LogBody bool
	// MaxBodyBytes จำกัดขนาด body ที่เขียนลง log เมื่อ LogBody เปิด
	MaxBodyBytes int
}

// maxBodyLogBytes ค่า default เมื่อเปิด LogBody
const maxBodyLogBytes = 4 << 10 // 4KB

// sensitiveFields คือ denylist รวมศูนย์
//
// ของเดิมใช้ regex ตัวเดียวครอบแค่ avatarData กับ token
// การรวมไว้ที่เดียวทำให้เพิ่มฟิลด์ใหม่ได้โดยไม่ต้องไปแก้ regex หลายที่
var sensitiveFields = []string{
	"avatardata", "avatar_data", "token", "password", "passwordhash",
	"password_hash", "secret", "authorization", "refreshtoken", "refresh_token",
	"microchipid", "microchip_id",
}

// NewAccessLog เขียน access log หนึ่งบรรทัดต่อหนึ่ง request ด้วย log/slog
func NewAccessLog(cfg LogConfig) fiber.Handler {
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = maxBodyLogBytes
	}

	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()

		attrs := []any{
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.Int("status", c.Response().StatusCode()),
			slog.Duration("latency", time.Since(start)),
			slog.String("request_id", c.Get(HeaderRequestID)),
			slog.String("ip", c.IP()),
		}
		if uid, ok := c.Locals("userId").(string); ok && uid != "" {
			attrs = append(attrs, slog.String("user_id", uid))
		}
		if v := c.Get("X-Source-System"); v != "" {
			attrs = append(attrs, slog.String("source_system", v))
		}
		if v := c.Get("X-Device-Id"); v != "" {
			attrs = append(attrs, slog.String("device_id", v))
		}

		if cfg.LogBody {
			if b := truncate(maskBody(c.Body()), cfg.MaxBodyBytes); b != "" {
				attrs = append(attrs, slog.String("req_body", b))
			}
			// ⚠️ ไม่ log response body ของ list endpoint
			// เพราะ GET /pets คืนสัตว์เลี้ยงทุกตัวพร้อม avatar
			if !isListPath(c.Path()) {
				if b := truncate(maskBody(c.Response().Body()), cfg.MaxBodyBytes); b != "" {
					attrs = append(attrs, slog.String("res_body", b))
				}
			}
		}

		level := slog.LevelInfo
		switch code := c.Response().StatusCode(); {
		case code >= 500:
			level = slog.LevelError
		case code >= 400:
			level = slog.LevelWarn
		}
		slog.LogAttrs(c.UserContext(), level, "http_request", toAttrs(attrs)...)

		return err
	}
}

func toAttrs(vals []any) []slog.Attr {
	out := make([]slog.Attr, 0, len(vals))
	for _, v := range vals {
		if a, ok := v.(slog.Attr); ok {
			out = append(out, a)
		}
	}
	return out
}

// isListPath บอกว่า path นี้คืน collection ซึ่ง body อาจใหญ่มาก
func isListPath(path string) bool {
	return strings.HasSuffix(path, "/pets") ||
		strings.HasSuffix(path, "-logs") ||
		strings.HasSuffix(path, "/caregivers")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…(ตัดทอน)"
}

// SetupLogger ตั้งค่า global logger เป็น JSON
func SetupLogger(level string) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})))
}
