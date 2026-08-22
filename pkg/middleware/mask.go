package middleware

import (
	"encoding/json"
	"strings"
)

const maskedValue = "[HIDDEN]"

// maskBody ซ่อนค่าของฟิลด์ที่อยู่ใน denylist
//
// parse เป็น JSON แล้วเดินโครงสร้างจริง แทนการใช้ regex
// เพราะ regex ครอบได้แค่รูปแบบที่คิดถึงตอนเขียน และพลาดง่ายเมื่อ
// ฟิลด์ซ้อนอยู่ในอาร์เรย์หรือ object ชั้นใน
func maskBody(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}

	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		// ไม่ใช่ JSON — ไม่รู้โครงสร้าง จึงไม่กล้าปล่อยผ่าน
		return "[ไม่ใช่ JSON]"
	}

	masked, _ := json.Marshal(maskValue(v))
	return string(masked)
}

func maskValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if isSensitive(k) {
				out[k] = maskedValue
				continue
			}
			out[k] = maskValue(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = maskValue(val)
		}
		return out
	default:
		return v
	}
}

func isSensitive(key string) bool {
	k := strings.ToLower(key)
	for _, s := range sensitiveFields {
		if k == s {
			return true
		}
	}
	return false
}
