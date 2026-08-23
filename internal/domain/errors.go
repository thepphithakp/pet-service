package domain

import "errors"

// Sentinel domain errors
var (
	ErrPetNotFound        = errors.New("pet not found")
	ErrCaregiverNotFound  = errors.New("caregiver not found")
	ErrLitterLogNotFound  = errors.New("litter log not found")
	ErrWaterLogNotFound   = errors.New("water log not found")
	ErrCaregiverDuplicate = errors.New("caregiver already exists for this pet")
	ErrInvalidID          = errors.New("invalid ID format")

	// ErrLogIDConflict เกิดเมื่อ client ส่ง id ที่ถูกใช้ไปแล้วกับสัตว์เลี้ยงตัวอื่น
	//
	// id มาจาก client ได้ (แพตเทิร์น optimistic update / offline sync)
	// การส่งซ้ำด้วย id เดิมกับสัตว์เลี้ยงตัวเดิมถือว่าปกติและได้แถวเดิมกลับไป
	// แต่ id ที่ไปชนของสัตว์เลี้ยงตัวอื่นแปลว่ามีอะไรผิด ต้องบอกให้ชัด
	// ไม่ใช่ปล่อยเป็น 500 ที่ไล่สาเหตุไม่ได้
	ErrLogIDConflict = errors.New("log id already used by another pet")

	// ErrUnauthenticated ไม่มี actor ใน context (ไม่ผ่าน auth middleware)
	ErrUnauthenticated = errors.New("unauthenticated")

	// ErrForbidden รู้ว่า actor เป็นใคร และเกี่ยวข้องกับ resource นี้ แต่สิทธิ์ไม่พอ
	//
	// ⚠️ ใช้เฉพาะเมื่อ actor "เห็น" resource นั้นได้อยู่แล้ว (เป็น caregiver แต่ permission ไม่พอ)
	//    ถ้า actor ไม่เกี่ยวข้องเลย ต้องคืน ErrPetNotFound แทน
	//    ไม่งั้น client จะแยกออกว่า UUID ไหนมีอยู่จริง แล้วไล่เดาได้
	ErrForbidden = errors.New("forbidden")

	// ErrInvalidPermission client ส่ง permission ID ที่ไม่มีใน master data
	ErrInvalidPermission = errors.New("invalid permission")

	// ErrValidation request ไม่ผ่านการตรวจโครงสร้าง
	ErrValidation = errors.New("validation failed")

	// ErrMasterDataNotFound ไม่พบรายการ master data ที่อ้างถึง
	ErrMasterDataNotFound = errors.New("master data not found")

	// ErrMasterDataDuplicate มี code นี้อยู่แล้ว
	ErrMasterDataDuplicate = errors.New("master data code already exists")

	// ErrVersionConflict มีคนอื่นแก้รายการนี้ไปแล้วระหว่างที่เรากำลังแก้
	ErrVersionConflict = errors.New("version conflict")
)
