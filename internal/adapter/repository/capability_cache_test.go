package repository

import "testing"

// TestCapabilityCache_Invalidate
//
// cache เก็บ role → capability ไว้ในหน่วยความจำ ถ้าแก้ master data
// แล้วไม่ล้าง สิทธิ์ที่เพิ่งเปลี่ยนจะยังไม่มีผลจนกว่า pod จะ restart
func TestCapabilityCache_Invalidate(t *testing.T) {
	r := &GORMCapabilityRepository{}
	r.cache = map[string]map[string]bool{"SUPER_ADMIN": {"PET_READ_ANY": true}}

	r.Invalidate()

	if r.cache != nil {
		t.Error("Invalidate ต้องล้าง cache ให้เป็น nil เพื่อให้โหลดใหม่รอบหน้า")
	}
}
