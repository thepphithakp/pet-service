package middleware

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMaskBody(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		mustHide   []string
		mustRemain []string
	}{
		{
			name:       "ฟิลด์ชั้นบนสุด",
			in:         `{"name":"มะลิ","avatarData":"AAAA","token":"secret"}`,
			mustHide:   []string{"AAAA", "secret"},
			mustRemain: []string{"มะลิ"},
		},
		{
			// regex เดิมพลาดเคสนี้เพราะครอบเฉพาะ pattern ที่คิดถึงตอนเขียน
			name:     "ฟิลด์ซ้อนอยู่ใน array ชั้นใน",
			in:       `{"pets":[{"name":"ส้ม","avatarData":"BBBB"}]}`,
			mustHide: []string{"BBBB"},
		},
		{
			name:     "ตัวพิมพ์ใหญ่เล็กต่างกัน",
			in:       `{"AvatarData":"CCCC","PASSWORD":"p"}`,
			mustHide: []string{"CCCC"},
		},
		{
			name:     "avatarData ที่เป็น object ไม่ใช่ string",
			in:       `{"avatarData":{"nested":"DDDD"}}`,
			mustHide: []string{"DDDD"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := maskBody([]byte(tc.in))
			if !json.Valid([]byte(got)) {
				t.Fatalf("ผลลัพธ์ต้องเป็น JSON ที่ valid: %s", got)
			}
			for _, s := range tc.mustHide {
				if strings.Contains(got, s) {
					t.Errorf("ต้องซ่อน %q แต่ยังอยู่ใน %s", s, got)
				}
			}
			for _, s := range tc.mustRemain {
				if !strings.Contains(got, s) {
					t.Errorf("%q ไม่ควรถูกซ่อน: %s", s, got)
				}
			}
		})
	}
}

func TestMaskBody_NonJSON(t *testing.T) {
	if got := maskBody([]byte("<xml>secret</xml>")); strings.Contains(got, "secret") {
		t.Fatalf("body ที่ไม่ใช่ JSON ต้องไม่ถูกปล่อยผ่าน: %s", got)
	}
	if maskBody(nil) != "" {
		t.Fatal("body ว่างต้องคืนสตริงว่าง")
	}
}

func TestIsListPath(t *testing.T) {
	for _, p := range []string{"/api/v1/pets", "/api/v1/pets/x/litter-logs", "/api/v1/pets/x/caregivers"} {
		if !isListPath(p) {
			t.Errorf("%s ควรถูกมองเป็น list path", p)
		}
	}
	if isListPath("/api/v1/pets/abc") {
		t.Error("path ของ resource เดี่ยวไม่ควรเป็น list path")
	}
}
