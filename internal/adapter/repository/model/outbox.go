package model

import (
	"time"

	"github.com/google/uuid"
)

// Outbox คือ event ที่รอส่งไป event-service
//
// เขียนลงตารางนี้ใน transaction เดียวกับข้อมูลธุรกิจ
// ถ้า transaction สำเร็จ event ต้องอยู่ครบเสมอ
type Outbox struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey"`

	EventType     string `gorm:"not null"`
	Action        string `gorm:"not null"`
	ActorID       string
	ActorUsername string
	EntityID      string
	EntityType    string
	// []byte + tag jsonb แทน datatypes.JSON เพื่อไม่ต้องดึง gorm.io/datatypes
	// ซึ่งลาก driver ของ MySQL เข้ามาด้วยทั้งที่ไม่ได้ใช้
	Payload []byte `gorm:"type:jsonb"`

	// IdempotencyKey ใช้ ID ของแถวนี้ จึงคงที่ตลอดไม่ว่าจะส่งกี่รอบ
	IdempotencyKey string `gorm:"not null"`

	CreatedAt time.Time `gorm:"not null;default:now()"`

	// PublishedAt เป็น nil = ยังไม่ได้ส่ง
	PublishedAt *time.Time

	Attempts  int `gorm:"not null;default:0"`
	LastError *string

	// NextAttemptAt ทำ exponential backoff โดยไม่ต้องมี scheduler แยก
	NextAttemptAt time.Time `gorm:"not null;default:now()"`
}

func (Outbox) TableName() string { return "event_outbox" }
