package domain

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// LogPageDefaultLimit / LogPageMaxLimit คุมขนาดหน้า
//
// max มีไว้กันผู้เรียกขอทีเดียวหมดตาราง ซึ่งทำให้ทั้ง database และ pod
// ต้องอุ้มข้อมูลก้อนใหญ่พร้อมกัน
const (
	LogPageDefaultLimit = 50
	LogPageMaxLimit     = 200
)

// LogPage คือคำขอหนึ่งหน้าของ log
type LogPage struct {
	Limit  int
	Cursor *LogCursor
}

// LogCursor ชี้ตำแหน่งที่จะอ่านต่อ
//
// ใช้ keyset (date, id) ไม่ใช่ offset เพราะ offset จะข้ามหรือซ้ำรายการ
// เมื่อมีการเพิ่ม log ใหม่ระหว่างที่ผู้ใช้กำลังเลื่อนดู — ซึ่งเกิดตลอดกับ log
// ที่เรียงจากใหม่ไปเก่า
//
// ต้องมี id เป็นตัวตัดสินคู่กับ date เพราะ log วันเดียวกันมีได้หลายรายการ
// ถ้าใช้ date อย่างเดียวจะวนซ้ำที่เดิมไม่จบ
type LogCursor struct {
	Date time.Time
	ID   uuid.UUID
}

// Encode ทำให้ cursor เป็นสตริงที่ client เก็บแล้วส่งกลับมาได้
//
// base64 ไม่ได้มีไว้ปกปิด แต่เพื่อบอกให้ชัดว่าเป็นค่าทึบ
// client ไม่ควรแกะไปตีความหรือประกอบขึ้นเอง
func (c LogCursor) Encode() string {
	raw := c.Date.UTC().Format(time.RFC3339Nano) + "|" + c.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeLogCursor แปลงกลับ พร้อมตรวจว่าใช้ได้จริง
func DecodeLogCursor(s string) (*LogCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%w: cursor ไม่ถูกต้อง", ErrValidation)
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("%w: cursor ไม่ถูกต้อง", ErrValidation)
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: cursor ไม่ถูกต้อง", ErrValidation)
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: cursor ไม่ถูกต้อง", ErrValidation)
	}
	return &LogCursor{Date: t, ID: id}, nil
}

// Normalize เติมค่า default และบังคับเพดาน
func (p LogPage) Normalize() LogPage {
	if p.Limit <= 0 {
		p.Limit = LogPageDefaultLimit
	}
	if p.Limit > LogPageMaxLimit {
		p.Limit = LogPageMaxLimit
	}
	return p
}

// encodeRawForTest ใช้ในเทสต์เพื่อสร้าง cursor ที่จงใจให้ผิดรูปแบบ
func (LogCursor) encodeRawForTest(raw string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}
