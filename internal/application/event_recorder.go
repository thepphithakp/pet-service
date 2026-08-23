package application

import (
	"context"

	"github.com/vertex/pet-service/internal/port"
)

// EventRecorder ผูกการเขียนข้อมูลธุรกิจกับการบันทึก event เข้าด้วยกัน
//
// เดิม service เขียนข้อมูลลง database แล้วค่อยยิง HTTP ไป event-service
// แบบ fire-and-forget สองอย่างนี้ไม่ได้อยู่ในหน่วยเดียวกัน ถ้า pod ถูกฆ่า
// หลัง commit แต่ก่อนยิงสำเร็จ event นั้นหายถาวรโดยไม่มีใครรู้
//
// ตอนนี้ event ถูกเขียนลงตาราง outbox ในทรานแซกชันเดียวกับข้อมูลธุรกิจ
// สำเร็จพร้อมกันหรือหายพร้อมกันเท่านั้น ไม่มีทางเหลื่อม
// แล้วมี worker แยกมาส่งทีหลัง
type EventRecorder struct {
	tx     port.TxManager
	outbox port.OutboxRepository
}

func NewEventRecorder(tx port.TxManager, outbox port.OutboxRepository) *EventRecorder {
	return &EventRecorder{tx: tx, outbox: outbox}
}

// Record รัน work ในทรานแซกชัน แล้วเขียน event ที่ work คืนมาลง outbox
// ในทรานแซกชันเดียวกัน
//
// คืน slice ว่างหรือ nil ได้เมื่อกรณีนั้นไม่ต้องบันทึกอะไร
// ถ้า work คืน error ทั้งข้อมูลและ event จะถูก rollback ไปด้วยกัน
func (r *EventRecorder) Record(ctx context.Context, work func(context.Context) ([]port.EventLog, error)) error {
	if r == nil || r.tx == nil || r.outbox == nil {
		// ไม่ได้ตั้ง recorder ไว้ (เช่นในเทสต์บางตัว) — ทำงานโดยไม่บันทึก event
		_, err := work(ctx)
		return err
	}

	return r.tx.Within(ctx, func(txCtx context.Context) error {
		events, err := work(txCtx)
		if err != nil {
			return err
		}
		for _, e := range events {
			if err := r.outbox.Enqueue(txCtx, e); err != nil {
				return err
			}
		}
		return nil
	})
}

// actorFrom ดึงข้อมูลผู้ทำจาก pointer ที่ service ถืออยู่
//
// รวมไว้ที่เดียวเพราะทุก service เขียนโค้ดชุดเดียวกันนี้ซ้ำๆ
func actorFrom(id, username *string) (string, string) {
	var actorID, actorName string
	if id != nil {
		actorID = *id
	}
	if username != nil {
		actorName = *username
	}
	return actorID, actorName
}
