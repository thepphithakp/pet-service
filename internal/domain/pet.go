package domain

import (
	"time"

	"github.com/google/uuid"
)

// Pet is the core domain entity. No framework tags.
type Pet struct {
	ID               uuid.UUID      `json:"id"`
	OwnerID          uuid.UUID      `json:"ownerId"`
	OwnerUsername    string         `json:"ownerUsername"`
	Name             string         `json:"name"`
	Species          string         `json:"species"`
	Breed            string         `json:"breed"`
	ColorCode        string         `json:"colorCode"`
	BirthDate        time.Time      `json:"birthDate"`
	Gender           string         `json:"gender"`
	AvatarData       []byte         `json:"avatarData,omitempty"`
	CurrentWeight    *float64       `json:"currentWeight,omitempty"`
	MicrochipId      *string        `json:"microchipId,omitempty"`
	IsSpayedNeutered bool           `json:"isSpayedNeutered"`
	BloodType        *string        `json:"bloodType,omitempty"`
	Allergies        *string        `json:"allergies,omitempty"`
	Personality      *string        `json:"personality,omitempty"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	CreatedBy        *string        `json:"createdBy,omitempty"`
	UpdatedBy        *string        `json:"updatedBy,omitempty"`
	Caregivers       []PetCaregiver `json:"caregivers,omitempty"`
}

// PetSummary คือสัตว์เลี้ยงหนึ่งตัวแบบไม่มีรูป ใช้กับ endpoint ที่คืนเป็นรายการ
//
// แยกออกมาเพราะ avatar_data เป็น bytea ที่ใหญ่มาก (บน production ตัวใหญ่สุด 2MB)
// การส่งไปกับทุกรายการทำให้ GET /pets ของผู้ใช้ที่มีสัตว์เลี้ยง 3 ตัว
// ต้องโหลดเกือบ 4MB (base64 ทำให้บวมอีก ~33%) ทั้งที่หน้าจอแสดงรูปเล็กๆ
//
// ผู้เรียกดูรูปผ่าน GET /pets/:id/avatar ซึ่ง cache ได้ด้วย ETag
type PetSummary struct {
	ID               uuid.UUID      `json:"id"`
	OwnerID          uuid.UUID      `json:"ownerId"`
	OwnerUsername    string         `json:"ownerUsername"`
	Name             string         `json:"name"`
	Species          string         `json:"species"`
	Breed            string         `json:"breed"`
	ColorCode        string         `json:"colorCode"`
	BirthDate        time.Time      `json:"birthDate"`
	Gender           string         `json:"gender"`
	CurrentWeight    *float64       `json:"currentWeight,omitempty"`
	MicrochipId      *string        `json:"microchipId,omitempty"`
	IsSpayedNeutered bool           `json:"isSpayedNeutered"`
	BloodType        *string        `json:"bloodType,omitempty"`
	Allergies        *string        `json:"allergies,omitempty"`
	Personality      *string        `json:"personality,omitempty"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	CreatedBy        *string        `json:"createdBy,omitempty"`
	UpdatedBy        *string        `json:"updatedBy,omitempty"`
	Caregivers       []PetCaregiver `json:"caregivers,omitempty"`

	// HasAvatar บอกว่ามีรูปให้ไปดึงไหม ผู้เรียกจะได้ไม่ต้องยิงเปล่า
	//
	// ไม่มี avatarUpdatedAt เพราะไม่มีคอลัมน์นั้นในฐานข้อมูล และไม่จำเป็น —
	// client เก็บ ETag ที่ได้จากครั้งก่อนแล้วส่ง If-None-Match มา
	// ถ้ารูปไม่เปลี่ยนจะได้ 304 ซึ่งไม่มี body เลย
	HasAvatar bool `json:"hasAvatar"`
}

// Avatar คือรูปของสัตว์เลี้ยงพร้อมข้อมูลที่ใช้ทำ HTTP cache
type Avatar struct {
	Data []byte
	// ETag คำนวณจากตัวข้อมูลรูป จึงเปลี่ยนเมื่อรูปเปลี่ยนเท่านั้น
	// ต่างจากการใช้ updated_at ของ pet ซึ่งเปลี่ยนทุกครั้งที่แก้ชื่อหรือน้ำหนัก
	ETag string
}
