package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/port"
)

// stubOutbox จำลอง outbox แบบเก็บใน memory
type stubOutbox struct {
	mu        sync.Mutex
	pending   []port.OutboxEvent
	published []uuid.UUID
	failed    []failure
}

type failure struct {
	id      uuid.UUID
	cause   string
	retryAt time.Time
}

func (s *stubOutbox) Enqueue(context.Context, port.EventLog) error { return nil }

func (s *stubOutbox) ClaimPending(_ context.Context, limit int) ([]port.OutboxEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) > limit {
		return s.pending[:limit], nil
	}
	out := s.pending
	s.pending = nil
	return out, nil
}

func (s *stubOutbox) MarkPublished(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.published = append(s.published, id)
	return nil
}

func (s *stubOutbox) MarkFailed(_ context.Context, id uuid.UUID, cause string, retryAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed = append(s.failed, failure{id: id, cause: cause, retryAt: retryAt})
	return nil
}

func (s *stubOutbox) CountPending(context.Context) (int64, error) { return 0, nil }
func (s *stubOutbox) DeletePublishedBefore(context.Context, time.Time) (int64, error) {
	return 0, nil
}

// stubSender บันทึกสิ่งที่ถูกส่งและควบคุมผลลัพธ์
type stubSender struct {
	mu   sync.Mutex
	sent []sentEvent
	err  error
}

type sentEvent struct {
	event          port.EventLog
	idempotencyKey string
}

func (s *stubSender) Send(_ context.Context, e port.EventLog, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.sent = append(s.sent, sentEvent{event: e, idempotencyKey: key})
	return nil
}

func newWorker(o port.OutboxRepository, s port.EventSender) *OutboxWorker {
	return NewOutboxWorker(o, passthroughTx{}, s, time.Second)
}

func TestOutboxWorker_SendsAndMarksPublished(t *testing.T) {
	id := uuid.New()
	out := &stubOutbox{pending: []port.OutboxEvent{{
		ID:             id,
		IdempotencyKey: id.String(),
		Event:          port.EventLog{EventType: "WaterLog", Action: "Water Intake Logged"},
	}}}
	sender := &stubSender{}

	if n := newWorker(out, sender).runOnce(context.Background()); n != 1 {
		t.Fatalf("ส่งสำเร็จ %d ตัว ต้องเป็น 1", n)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("ส่งออกไป %d ครั้ง ต้องเป็น 1", len(sender.sent))
	}
	if sender.sent[0].idempotencyKey != id.String() {
		t.Errorf("idempotency key = %q ต้องเป็น id ของแถว %q",
			sender.sent[0].idempotencyKey, id)
	}
	if len(out.published) != 1 || out.published[0] != id {
		t.Errorf("ต้อง mark ว่าส่งแล้ว: %v", out.published)
	}
	if len(out.failed) != 0 {
		t.Errorf("ไม่ควรมีรายการล้มเหลว: %v", out.failed)
	}
}

// TestOutboxWorker_FailureSchedulesRetry
//
// ปลายทางล่มต้องไม่ทำให้ event หาย — ต้องถูกเลื่อนไปลองใหม่
func TestOutboxWorker_FailureSchedulesRetry(t *testing.T) {
	id := uuid.New()
	out := &stubOutbox{pending: []port.OutboxEvent{{ID: id, IdempotencyKey: id.String()}}}
	sender := &stubSender{err: errors.New("event-service ล่ม")}

	if n := newWorker(out, sender).runOnce(context.Background()); n != 0 {
		t.Fatalf("ไม่ควรมีตัวไหนสำเร็จ ได้ %d", n)
	}
	if len(out.published) != 0 {
		t.Error("ห้าม mark ว่าส่งแล้วเมื่อส่งไม่สำเร็จ")
	}
	if len(out.failed) != 1 {
		t.Fatalf("ต้องบันทึกความล้มเหลว: %v", out.failed)
	}
	if out.failed[0].retryAt.Before(time.Now()) {
		t.Error("ต้องเลื่อนเวลาลองใหม่ไปข้างหน้า")
	}
	if out.failed[0].cause == "" {
		t.Error("ต้องเก็บสาเหตุไว้ให้ไล่ปัญหาได้")
	}
}

// TestOutboxWorker_MarkPublishedFailureIsNotCountedAsSent
//
// ส่งสำเร็จแต่บันทึกสถานะไม่ได้ = ยังไม่จบ ต้องส่งซ้ำรอบหน้า
// ซึ่งปลอดภัยเพราะมี idempotency key
func TestOutboxWorker_MarkPublishedFailureIsNotCountedAsSent(t *testing.T) {
	id := uuid.New()
	out := &failingMarkOutbox{stubOutbox: stubOutbox{
		pending: []port.OutboxEvent{{ID: id, IdempotencyKey: id.String()}},
	}}

	if n := newWorker(out, &stubSender{}).runOnce(context.Background()); n != 0 {
		t.Errorf("บันทึกสถานะไม่ได้ต้องไม่นับว่าสำเร็จ ได้ %d", n)
	}
}

type failingMarkOutbox struct{ stubOutbox }

func (f *failingMarkOutbox) MarkPublished(context.Context, uuid.UUID) error {
	return errors.New("เขียนไม่ได้")
}

// TestBackoff_GrowsAndIsCapped
func TestBackoff_GrowsAndIsCapped(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{1, 5 * time.Second},
		{2, 10 * time.Second},
		{3, 20 * time.Second},
		{4, 40 * time.Second},
	}
	for _, tc := range cases {
		if got := backoffFor(tc.attempts); got != tc.want {
			t.Errorf("attempts=%d → %v ต้องเป็น %v", tc.attempts, got, tc.want)
		}
	}

	// ต้องมีเพดาน ไม่งั้น event ที่ค้างนานจะรอเป็นวันหลังปลายทางกลับมาแล้ว
	if got := backoffFor(100); got != outboxMaxBackoff {
		t.Errorf("attempts=100 → %v ต้องชนเพดาน %v", got, outboxMaxBackoff)
	}
}

// TestOutboxWorker_StopsOnContextCancel
func TestOutboxWorker_StopsOnContextCancel(t *testing.T) {
	w := NewOutboxWorker(&stubOutbox{}, passthroughTx{}, &stubSender{}, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker ไม่หยุดเมื่อ context ถูกยกเลิก — pod จะปิดตัวไม่ลง")
	}
}
