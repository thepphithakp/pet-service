package application

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/vertex/pet-service/internal/port"
)

// ค่าตั้งของ worker
const (
	// outboxBatchSize จำกัดจำนวนที่หยิบต่อรอบ
	// ใหญ่เกินไปจะถือ lock นานและกิน memory โดยไม่จำเป็น
	outboxBatchSize = 20

	// outboxMaxAttempts เลิกพยายามหลังจากนี้
	//
	// ไม่ลบทิ้งและไม่ mark ว่าสำเร็จ — ปล่อยค้างไว้ให้เห็นใน metric
	// เพราะ event ที่ส่งไม่ได้คือปัญหาที่คนต้องมาดู ไม่ใช่ขยะที่ควรซ่อน
	outboxMaxAttempts = 12

	outboxBaseBackoff = 5 * time.Second
	outboxMaxBackoff  = 30 * time.Minute
)

// OutboxWorker ส่ง event ที่ค้างใน outbox ออกไป
//
// แยกจากเส้นทาง request โดยสิ้นเชิง — request ของผู้ใช้แค่เขียน event
// ลง outbox ใน transaction เดียวกับข้อมูลธุรกิจแล้วจบ ไม่ต้องรอ HTTP
type OutboxWorker struct {
	repo     port.OutboxRepository
	tx       port.TxManager
	sender   port.EventSender
	interval time.Duration

	// now แยกออกมาเพื่อให้เทสต์คุมเวลาได้
	now func() time.Time

	// observe รายงานสถานะให้ระบบ metric
	//
	// รับเป็น callback เพื่อให้ชั้น application ไม่ต้องรู้จัก Prometheus
	// ซึ่งเป็นรายละเอียดของ adapter ไม่ใช่ของโดเมน
	observe OutboxObserver
}

// OutboxObserver รับรายงานสถานะของ outbox
type OutboxObserver struct {
	SetPending func(int64)
	Delivery   func(result string)
}

// WithObserver ผูกตัวรายงาน metric เข้ากับ worker
func (w *OutboxWorker) WithObserver(o OutboxObserver) *OutboxWorker {
	w.observe = o
	return w
}

func (w *OutboxWorker) reportPending(ctx context.Context) {
	if w.observe.SetPending == nil {
		return
	}
	n, err := w.repo.CountPending(ctx)
	if err != nil {
		return
	}
	w.observe.SetPending(n)
}

func (w *OutboxWorker) reportDelivery(result string) {
	if w.observe.Delivery != nil {
		w.observe.Delivery(result)
	}
}

func NewOutboxWorker(repo port.OutboxRepository, tx port.TxManager, sender port.EventSender, interval time.Duration) *OutboxWorker {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &OutboxWorker{repo: repo, tx: tx, sender: sender, interval: interval, now: time.Now}
}

// Run วนทำงานจนกว่า ctx จะถูกยกเลิก
//
// เรียกใน goroutine ตอน start และ cancel ctx ตอนปิดตัว
func (w *OutboxWorker) Run(ctx context.Context) {
	slog.Info("outbox worker เริ่มทำงาน", "interval", w.interval)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		// ทำหนึ่งรอบทันทีตอนเริ่ม ไม่ต้องรอ tick แรก
		// เผื่อมี event ค้างจากตอน pod ก่อนหน้าถูกปิด
		w.runOnce(ctx)

		select {
		case <-ctx.Done():
			slog.Info("outbox worker หยุดทำงาน")
			return
		case <-ticker.C:
		}
	}
}

// runOnce ทำงานหนึ่งรอบ — คืนจำนวนที่ส่งสำเร็จ
func (w *OutboxWorker) runOnce(ctx context.Context) int {
	var sent int

	err := w.tx.Within(ctx, func(txCtx context.Context) error {
		events, err := w.repo.ClaimPending(txCtx, outboxBatchSize)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}

		for _, e := range events {
			if w.deliver(txCtx, e) {
				sent++
				w.reportDelivery("success")
			} else {
				w.reportDelivery("failure")
			}
		}
		return nil
	})

	if err != nil && !errors.Is(err, context.Canceled) {
		slog.ErrorContext(ctx, "รอบการส่ง outbox ล้มเหลว", "error", err)
	}

	// รายงานจำนวนที่ค้างทุกรอบ ไม่ว่ารอบนี้จะมีงานหรือไม่
	// เพื่อให้ metric สะท้อนสถานะจริงเสมอ ไม่ใช่ค้างค่าเก่าไว้
	w.reportPending(ctx)
	return sent
}

// deliver ส่ง event หนึ่งตัวแล้วบันทึกผล — คืน true เมื่อสำเร็จ
func (w *OutboxWorker) deliver(ctx context.Context, e port.OutboxEvent) bool {
	// ⚠️ ctx ตรงนี้ผูกกับ transaction ที่ถือ lock อยู่
	//    การยิง HTTP ระหว่างถือ lock ทำให้ lock ถูกถือนานเท่าเวลาที่ปลายทางตอบ
	//    ยอมรับได้เพราะ HTTP client มี timeout และ batch เล็ก
	//    ถ้าวันหนึ่งปริมาณสูงขึ้นมาก ให้แยกเป็น claim → ปิด transaction → ส่ง → อัปเดต
	err := w.sender.Send(ctx, e.Event, e.IdempotencyKey)
	if err == nil {
		if markErr := w.repo.MarkPublished(ctx, e.ID); markErr != nil {
			slog.ErrorContext(ctx, "ส่ง event สำเร็จแต่บันทึกสถานะไม่ได้",
				"outbox_id", e.ID, "error", markErr)
			// ไม่ถือว่าสำเร็จ — รอบหน้าจะส่งซ้ำ ซึ่งปลอดภัยเพราะมี idempotency key
			return false
		}
		return true
	}

	attempts := e.Attempts + 1
	retryAt := w.now().Add(backoffFor(attempts))

	level := slog.LevelWarn
	if attempts >= outboxMaxAttempts {
		// ถึงเพดานแล้ว — ยังคงเลื่อนเวลาต่อไปเรื่อยๆ ด้วย backoff สูงสุด
		// แต่ยกระดับ log เป็น ERROR เพื่อให้เห็นว่ามีของค้างที่ต้องมาดู
		level = slog.LevelError
	}
	slog.Log(ctx, level, "ส่ง event ไม่สำเร็จ",
		"outbox_id", e.ID, "event_type", e.Event.EventType,
		"attempts", attempts, "retry_at", retryAt, "error", err)

	if markErr := w.repo.MarkFailed(ctx, e.ID, err.Error(), retryAt); markErr != nil {
		slog.ErrorContext(ctx, "บันทึกความล้มเหลวของ outbox ไม่ได้",
			"outbox_id", e.ID, "error", markErr)
	}
	return false
}

// backoffFor คำนวณเวลารอแบบทวีคูณ
//
// 5s, 10s, 20s, 40s ... จนชนเพดาน 30 นาที
// มีเพดานเพื่อไม่ให้ event ที่ค้างนานกลายเป็นรอเป็นวันหลังปลายทางกลับมาแล้ว
func backoffFor(attempts int) time.Duration {
	d := outboxBaseBackoff
	for i := 1; i < attempts; i++ {
		d *= 2
		if d >= outboxMaxBackoff {
			return outboxMaxBackoff
		}
	}
	return d
}
