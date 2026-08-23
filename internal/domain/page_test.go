package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestLogCursor_RoundTrip(t *testing.T) {
	// เวลาที่มีเศษนาโนวินาที — ถ้า encode ทำหายจะเลื่อนตำแหน่งไปทั้งหน้า
	want := LogCursor{
		Date: time.Date(2026, 8, 23, 10, 30, 45, 123456789, time.UTC),
		ID:   uuid.MustParse("1b17cbe6-32c2-4261-97e5-546ec2a723f1"),
	}

	got, err := DecodeLogCursor(want.Encode())
	if err != nil {
		t.Fatalf("decode ไม่สำเร็จ: %v", err)
	}
	if !got.Date.Equal(want.Date) {
		t.Errorf("Date = %v ต้องเป็น %v — ความละเอียดหายระหว่างทาง", got.Date, want.Date)
	}
	if got.ID != want.ID {
		t.Errorf("ID = %v ต้องเป็น %v", got.ID, want.ID)
	}
}

func TestDecodeLogCursor_RejectsGarbage(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"ไม่ใช่ base64", "!!!"},
		{"base64 แต่ไม่มีตัวคั่น", "aGVsbG8"},
		{"วันที่อ่านไม่ออก", encodeRaw("ไม่ใช่วันที่|1b17cbe6-32c2-4261-97e5-546ec2a723f1")},
		{"uuid อ่านไม่ออก", encodeRaw("2026-08-23T10:00:00Z|ไม่ใช่ uuid")},
		{"ว่างเปล่า", encodeRaw("")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeLogCursor(tc.input)
			if err == nil {
				t.Fatal("ต้องคืน error")
			}
			// ต้องเป็น ErrValidation เพื่อให้ handler ตอบ 400 ไม่ใช่ 500
			if !errors.Is(err, ErrValidation) {
				t.Errorf("ต้องเป็น ErrValidation เพื่อให้ตอบ 400: %v", err)
			}
		})
	}
}

func TestLogPage_Normalize(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		want  int
	}{
		{"ไม่ระบุ → default", 0, LogPageDefaultLimit},
		{"ติดลบ → default", -10, LogPageDefaultLimit},
		{"เกินเพดาน → ถูกตัด", LogPageMaxLimit + 5000, LogPageMaxLimit},
		{"ปกติ → ตามที่ขอ", 25, 25},
		{"เท่าเพดานพอดี", LogPageMaxLimit, LogPageMaxLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := (LogPage{Limit: tc.limit}).Normalize().Limit; got != tc.want {
				t.Errorf("Limit = %d ต้องเป็น %d", got, tc.want)
			}
		})
	}
}

func encodeRaw(s string) string {
	return LogCursor{}.encodeRawForTest(s)
}
