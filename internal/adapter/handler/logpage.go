package handler

import (
	"log/slog"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/pkg/apperror"
)

// LogPageResponse คือรูปแบบที่คืนเมื่อผู้เรียกขอแบบแบ่งหน้า
//
// data เป็น array ตัวเดิมทุกอย่าง เปลี่ยนแค่ห่อไว้ในกล่องพร้อมข้อมูลหน้า
type LogPageResponse struct {
	Data       any     `json:"data"`
	NextCursor *string `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
}

// unpaginatedWarnThreshold คือจำนวนที่ถือว่ามากพอจะเตือน
//
// ไม่ได้ตัดข้อมูลทิ้ง — แค่ทำให้เห็นใน log ว่ามีผู้เรียกที่ยังดึงทีเดียวหมด
// จะได้รู้ว่าถึงเวลาต้องผลักดันให้ client เปลี่ยนไปใช้แบบแบ่งหน้าแล้ว
const unpaginatedWarnThreshold = 500

// wantsPage บอกว่าผู้เรียกขอแบบแบ่งหน้าไหม
//
// ตัดสินจาก "มี limit หรือ cursor ในคำขอไหม" ไม่ใช่จากค่า default
// ผู้เรียกเดิมที่ไม่ส่งอะไรมาจะได้ array แบบเดิมเป๊ะ ไม่มีอะไรเปลี่ยน
func wantsPage(c *fiber.Ctx) bool {
	return c.Query("limit") != "" || c.Query("cursor") != ""
}

// parseLogPage อ่าน limit กับ cursor จาก query string
func parseLogPage(c *fiber.Ctx) (domain.LogPage, error) {
	var page domain.LogPage

	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return page, apperror.BadRequest("limit ต้องเป็นจำนวนเต็มบวก", nil)
		}
		page.Limit = n
	}

	if raw := c.Query("cursor"); raw != "" {
		cur, err := domain.DecodeLogCursor(raw)
		if err != nil {
			return page, apperror.FromDomain(err)
		}
		page.Cursor = cur
	}

	return page.Normalize(), nil
}

// nextCursorFrom สร้าง cursor ของหน้าถัดไปจากรายการสุดท้ายที่ส่งไป
//
// คืน nil เมื่อไม่มีหน้าถัดไป เพื่อให้ client เช็คได้จากค่าเดียว
func nextCursorFrom(hasMore bool, last *domain.LogCursor) *string {
	if !hasMore || last == nil {
		return nil
	}
	s := last.Encode()
	return &s
}

// warnIfLargeUnpaginated เตือนเมื่อมีผู้เรียกดึงทีเดียวหมดและได้ผลลัพธ์เยอะ
func warnIfLargeUnpaginated(c *fiber.Ctx, n int) {
	if n < unpaginatedWarnThreshold {
		return
	}
	slog.WarnContext(c.UserContext(),
		"คืน log ทีเดียวจำนวนมากเพราะผู้เรียกไม่ได้ขอแบบแบ่งหน้า",
		"count", n, "path", c.Path(),
		"hint", "ให้ client ส่ง ?limit= มาเพื่อแบ่งหน้า")
}
