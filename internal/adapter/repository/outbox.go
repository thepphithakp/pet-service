package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/vertex/pet-service/internal/adapter/repository/model"
	"github.com/vertex/pet-service/internal/port"
)

type GORMOutboxRepository struct {
	db *gorm.DB
}

func NewGORMOutboxRepository(db *gorm.DB) *GORMOutboxRepository {
	return &GORMOutboxRepository{db: db}
}

// Enqueue เขียน event ลง outbox
//
// ใช้ dbFrom เพื่อให้เขียนอยู่ใน transaction เดียวกับข้อมูลธุรกิจ
// ถ้าเรียกนอก transaction จะกลายเป็นการเขียนแยก ซึ่งเสียจุดประสงค์ทั้งหมด
// — ผู้เรียกต้องอยู่ใน TxManager.Within เสมอ
func (r *GORMOutboxRepository) Enqueue(ctx context.Context, e port.EventLog) error {
	payload, err := marshalPayload(e.Payload)
	if err != nil {
		return err
	}

	id := uuid.New()
	row := model.Outbox{
		ID:            id,
		EventType:     e.EventType,
		Action:        e.Action,
		ActorID:       e.ActorID,
		ActorUsername: e.ActorUsername,
		EntityID:      e.EntityID,
		EntityType:    e.EntityType,
		Payload:       payload,
		// ใช้ id ของแถวเป็น idempotency key — คงที่ตลอดทุกครั้งที่ retry
		IdempotencyKey: id.String(),
	}
	return dbFrom(ctx, r.db).Create(&row).Error
}

// ClaimPending จับจอง event ที่ถึงเวลาส่งแล้ว
//
// FOR UPDATE SKIP LOCKED ทำให้หลาย replica ทำงานพร้อมกันได้โดยไม่หยิบชนกัน
// replica ที่มาทีหลังจะข้ามแถวที่ถูกจองอยู่ไปหยิบตัวถัดไปแทนที่จะรอ
//
// ต้องเรียกภายใน transaction — lock จะถูกปล่อยตอน transaction จบ
func (r *GORMOutboxRepository) ClaimPending(ctx context.Context, limit int) ([]port.OutboxEvent, error) {
	var rows []model.Outbox
	err := dbFrom(ctx, r.db).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("published_at IS NULL AND next_attempt_at <= ?", time.Now()).
		Order("created_at").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]port.OutboxEvent, len(rows))
	for i, row := range rows {
		out[i] = port.OutboxEvent{
			ID:             row.ID,
			Attempts:       row.Attempts,
			IdempotencyKey: row.IdempotencyKey,
			Event: port.EventLog{
				EventType:     row.EventType,
				Action:        row.Action,
				ActorID:       row.ActorID,
				ActorUsername: row.ActorUsername,
				EntityID:      row.EntityID,
				EntityType:    row.EntityType,
				Payload:       unmarshalPayload(row.Payload),
			},
		}
	}
	return out, nil
}

// MarkPublished บันทึกว่าส่งสำเร็จแล้ว
func (r *GORMOutboxRepository) MarkPublished(ctx context.Context, id uuid.UUID) error {
	return dbFrom(ctx, r.db).
		Model(&model.Outbox{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"published_at": time.Now(),
			"last_error":   nil,
		}).Error
}

// MarkFailed บันทึกความล้มเหลวและเลื่อนเวลาลองใหม่
func (r *GORMOutboxRepository) MarkFailed(ctx context.Context, id uuid.UUID, cause string, retryAt time.Time) error {
	return dbFrom(ctx, r.db).
		Model(&model.Outbox{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"attempts":        gorm.Expr("attempts + 1"),
			"last_error":      truncateError(cause),
			"next_attempt_at": retryAt,
		}).Error
}

// CountPending นับงานที่ค้างอยู่ ใช้ทำ metric
func (r *GORMOutboxRepository) CountPending(ctx context.Context) (int64, error) {
	var n int64
	err := dbFrom(ctx, r.db).
		Model(&model.Outbox{}).
		Where("published_at IS NULL").
		Count(&n).Error
	return n, err
}

// DeletePublishedBefore ล้างแถวที่ส่งไปแล้วและเก่าเกินกำหนด
//
// ตารางนี้เป็น append-only ถ้าไม่ลบจะโตไปเรื่อยๆ จนกระทบ query อื่น
func (r *GORMOutboxRepository) DeletePublishedBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	tx := dbFrom(ctx, r.db).
		Where("published_at IS NOT NULL AND published_at < ?", cutoff).
		Delete(&model.Outbox{})
	return tx.RowsAffected, tx.Error
}

// maxErrorLength กันข้อความ error ยาวๆ จาก database ทำให้แถวบวม
const maxErrorLength = 500

func truncateError(s string) string {
	if len(s) <= maxErrorLength {
		return s
	}
	return s[:maxErrorLength] + "…"
}

func marshalPayload(p map[string]any) ([]byte, error) {
	if p == nil {
		return nil, nil
	}
	return jsonMarshal(p)
}

func unmarshalPayload(j []byte) map[string]any {
	if len(j) == 0 {
		return nil
	}
	var m map[string]any
	if err := jsonUnmarshal(j, &m); err != nil {
		return nil
	}
	return m
}
