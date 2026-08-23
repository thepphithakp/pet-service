package port

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type EventLog struct {
	EventType     string                 `json:"eventType"`
	Action        string                 `json:"action"`
	ActorID       string                 `json:"actorId"`
	ActorUsername string                 `json:"actorUsername"`
	EntityID      string                 `json:"entityId"`
	EntityType    string                 `json:"entityType"`
	Payload       map[string]interface{} `json:"payload"`
}

type EventPublisher interface {
	Publish(ctx context.Context, event EventLog)
}

// OutboxEvent คือ event หนึ่งแถวที่ worker หยิบมาส่ง
type OutboxEvent struct {
	ID             uuid.UUID
	Attempts       int
	IdempotencyKey string
	Event          EventLog
}

// OutboxRepository เก็บ event ที่รอส่ง
//
// Enqueue ต้องถูกเรียกภายใน transaction เดียวกับข้อมูลธุรกิจเสมอ
// ไม่งั้นจะกลับไปเป็นปัญหาเดิมคือข้อมูลกับ event ไม่ไปด้วยกัน
type OutboxRepository interface {
	Enqueue(ctx context.Context, event EventLog) error

	// ClaimPending จับจองงานด้วย FOR UPDATE SKIP LOCKED
	// ต้องเรียกภายใน transaction — lock ปล่อยตอน transaction จบ
	ClaimPending(ctx context.Context, limit int) ([]OutboxEvent, error)

	MarkPublished(ctx context.Context, id uuid.UUID) error
	MarkFailed(ctx context.Context, id uuid.UUID, cause string, retryAt time.Time) error

	CountPending(ctx context.Context) (int64, error)
	DeletePublishedBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

// TxManager รัน function ใน transaction เดียว
//
// มีเพื่อให้ชั้น application สั่งขอบเขต transaction ได้โดยไม่ต้องรู้จัก GORM
type TxManager interface {
	Within(ctx context.Context, fn func(context.Context) error) error
}

// EventSender ส่ง event ออกไปจริง — แยกจาก EventPublisher เพื่อให้ worker
// เรียกได้ตรงๆ และคืน error ให้ตัดสินใจว่าจะ retry ไหม
type EventSender interface {
	Send(ctx context.Context, event EventLog, idempotencyKey string) error
}
