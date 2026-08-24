package middleware

import (
	"errors"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/vertex/pet-service/pkg/apperror"
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

// infraPaths คือ endpoint ที่ถูกเรียกโดย k8s / Prometheus ไม่ใช่ผู้ใช้
//
// ไม่ log เพราะ probe ยิงทุก 2–5 วินาทีตลอดเวลา ถ้า log ด้วยจะกลบ
// access log ของ request จริงจนหาไม่เจอ (เจอปัญหานี้ตอนไล่ incident จริง)
// ส่วน error/สถานะที่ผิดปกติยังเห็นได้จาก probe ของ k8s เองอยู่แล้ว
var infraPaths = map[string]bool{
	"/livez":   true,
	"/readyz":  true,
	"/health":  true,
	"/metrics": true,
}

// IsInfraPath บอกว่า path นี้เป็นของ infrastructure ไม่ใช่ traffic ของผู้ใช้
func IsInfraPath(path string) bool { return infraPaths[path] }

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
		if IsInfraPath(c.Path()) {
			return c.Next()
		}

		start := time.Now()
		err := c.Next()

		// 🔴 อ่าน c.Response().StatusCode() ตรงๆ ไม่พอเมื่อ handler
		// return error object (apperror.AppError, fiber.Error) แทนที่จะ
		// เรียก c.Status().JSON() เอง
		//
		// Fiber เรียก ErrorHandler กลาง "หลังจาก" middleware chain (รวม
		// access log นี้) unwind กลับไปแล้ว ไม่ใช่ระหว่าง c.Next() —
		// ดังนั้นถ้า handler แค่ return err การอ่าน status ตรงนี้จะยังเป็น
		// ค่า default 200 ของ Fiber อยู่ ทั้งที่ผู้เรียกจริงจะได้ status
		// ที่ ErrorHandler กำหนดทีหลัง (ซึ่งส่วนใหญ่ในโค้ดนี้คือ error
		// จาก apperror.* — วิธี return error ที่ handler ส่วนใหญ่ใช้)
		//
		// แก้โดยเดาผลลัพธ์ที่ ErrorHandler จะให้ล่วงหน้า ด้วย logic
		// เดียวกับใน error_handler.go ทุกประการ
		status := c.Response().StatusCode()
		if err != nil {
			status = resolveErrStatus(err)
		}

		// endpoint คือ path จริงที่แทน UUID ด้วย :id แล้ว เช่น
		// "/api/v1/pets/86715873-..." กลายเป็น "/api/v1/pets/:id"
		//
		// 🔴 เดิมใช้ c.Route().Path (route pattern ที่ Fiber ลงทะเบียนไว้)
		// แต่ auth middleware ของ pet-service ถูกผูกไว้ที่ระดับ group
		// (app.Group("/api/v1", authMW)) พอ auth ปฏิเสธก่อนถึง route ย่อย
		// Fiber ยังไม่ทันเดินไปถึง route เต็ม c.Route() จึงได้แค่ path
		// ของ group ("/api/v1") ไม่ใช่ "/api/v1/pets/:id" — เกิดกับทุก
		// request ที่ถูกปฏิเสธที่ auth ซึ่งเป็นสัดส่วนใหญ่ของ error จริง
		//
		// แก้ด้วยการ normalize UUID ออกจาก path ตรงๆ แทน ไม่พึ่งว่า
		// Fiber จะ resolve ไปถึง route ไหนแล้วตอนที่ error เกิด
		endpoint := normalizeEndpoint(c.Path())

		attrs := []any{
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.String("endpoint", endpoint),
			slog.Int("status", status),
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

		// 🔴 log body เสมอเมื่อ error (status >= 400) ไม่ต้องเปิด cfg.LogBody
		//
		// ตอน investigate ปัญหาจริง (VT-69) สิ่งที่ขาดคือ body ตอน error
		// ไม่ใช่ตอนทำงานปกติ — เปิด log body ทุก request จะทำให้ log บวม
		// เร็วมากเพราะ traffic ปกติมากกว่า error หลายเท่า และเพิ่มความเสี่ยง
		// ข้อมูลส่วนตัวไหลเข้า log โดยไม่จำเป็น จึงจำกัดไว้เฉพาะตอนพังเท่านั้น
		//
		// cfg.LogBody ยังใช้ได้เหมือนเดิมสำหรับเปิด log ทุก request ตอน
		// debug ที่ non-production
		logBody := cfg.LogBody || status >= 400
		if logBody {
			if b := truncate(maskBody(c.Body()), cfg.MaxBodyBytes); b != "" {
				attrs = append(attrs, slog.String("req_body", b))
			}
			// ⚠️ ไม่ log response body ของ list endpoint แม้จะ error
			// เพราะ GET /pets คืนสัตว์เลี้ยงทุกตัวพร้อม avatar
			if !isListPath(c.Path()) {
				if b := truncate(maskBody(c.Response().Body()), cfg.MaxBodyBytes); b != "" {
					attrs = append(attrs, slog.String("res_body", b))
				}
			}
		}

		level := slog.LevelInfo
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
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

// resolveErrStatus เดา HTTP status ที่ ErrorHandler จะกำหนดให้ error นี้
// ต้องตรงกับ logic ใน error_handler.go ทุกประการ ไม่งั้น access log
// กับ response จริงจะไม่ตรงกัน
func resolveErrStatus(err error) int {
	var appErr *apperror.AppError
	if apperror.IsAppError(err, &appErr) {
		return appErr.Code
	}
	var fe *fiber.Error
	if errors.As(err, &fe) {
		return fe.Code
	}
	return fiber.StatusInternalServerError
}

// uuidSegment จับ UUID มาตรฐาน (8-4-4-4-12 hex) ไม่สนตัวพิมพ์ใหญ่เล็ก
var uuidSegment = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// normalizeEndpoint แทนที่ segment ที่เป็น UUID ด้วย ":id"
//
// ทำงานอิสระจากการ resolve route ของ Fiber โดยสิ้นเชิง จึงถูกต้องเสมอ
// ไม่ว่า request จะถูกปฏิเสธที่ชั้นไหนของ middleware chain
func normalizeEndpoint(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if uuidSegment.MatchString(p) {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}
