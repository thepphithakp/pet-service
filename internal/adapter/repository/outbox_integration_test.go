//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/port"
)

func TestOutboxRepository_CountPendingAndCleanup(t *testing.T) {
	db := openDB(t)
	repo := NewGORMOutboxRepository(db)
	tx := NewGORMTxManager(db)
	ctx := context.Background()

	marker := "outbox-repo-test-" + uuid.NewString()
	t.Cleanup(func() { db.Exec("DELETE FROM event_outbox WHERE entity_id = ?", marker) })

	before, err := repo.CountPending(ctx)
	if err != nil {
		t.Fatalf("CountPending ไม่สำเร็จ: %v", err)
	}

	// เขียนสามตัว
	for i := 0; i < 3; i++ {
		if err := repo.Enqueue(ctx, port.EventLog{
			EventType: "RepoTest", Action: "enqueue",
			EntityType: "Pet", EntityID: marker,
			Payload: map[string]any{"i": i},
		}); err != nil {
			t.Fatalf("Enqueue ไม่สำเร็จ: %v", err)
		}
	}

	after, err := repo.CountPending(ctx)
	if err != nil {
		t.Fatalf("CountPending ไม่สำเร็จ: %v", err)
	}
	if after-before != 3 {
		t.Fatalf("ค้างเพิ่ม %d ตัว ต้องเป็น 3", after-before)
	}

	// จับจองแล้ว mark ว่าส่งแล้วทั้งหมด
	err = tx.Within(ctx, func(txCtx context.Context) error {
		events, err := repo.ClaimPending(txCtx, 50)
		if err != nil {
			return err
		}
		for _, e := range events {
			if e.Event.EntityID != marker {
				continue
			}
			if e.IdempotencyKey != e.ID.String() {
				t.Errorf("idempotency key ต้องเป็น id ของแถว: %q vs %q", e.IdempotencyKey, e.ID)
			}
			if e.Event.Payload == nil {
				t.Error("payload ต้องถูกอ่านกลับมาได้")
			}
			if err := repo.MarkPublished(txCtx, e.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("จับจอง/บันทึกไม่สำเร็จ: %v", err)
	}

	nowPending, _ := repo.CountPending(ctx)
	if nowPending != before {
		t.Errorf("ส่งครบแล้วต้องเหลือค้างเท่าเดิม (%d) ได้ %d", before, nowPending)
	}

	// ล้างแถวที่ส่งแล้ว
	deleted, err := repo.DeletePublishedBefore(ctx, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("DeletePublishedBefore ไม่สำเร็จ: %v", err)
	}
	if deleted < 3 {
		t.Errorf("ลบไป %d แถว ต้องลบอย่างน้อย 3", deleted)
	}

	var left int64
	db.Raw("SELECT count(*) FROM event_outbox WHERE entity_id = ?", marker).Scan(&left)
	if left != 0 {
		t.Errorf("ยังเหลือ %d แถวที่ควรถูกล้าง", left)
	}
}

// TestOutboxRepository_MarkFailedSchedulesRetry
func TestOutboxRepository_MarkFailedSchedulesRetry(t *testing.T) {
	db := openDB(t)
	repo := NewGORMOutboxRepository(db)
	tx := NewGORMTxManager(db)
	ctx := context.Background()

	marker := "outbox-retry-test-" + uuid.NewString()
	t.Cleanup(func() { db.Exec("DELETE FROM event_outbox WHERE entity_id = ?", marker) })

	if err := repo.Enqueue(ctx, port.EventLog{
		EventType: "RepoTest", Action: "retry", EntityType: "Pet", EntityID: marker,
	}); err != nil {
		t.Fatalf("Enqueue ไม่สำเร็จ: %v", err)
	}

	retryAt := time.Now().Add(10 * time.Minute)
	err := tx.Within(ctx, func(txCtx context.Context) error {
		events, err := repo.ClaimPending(txCtx, 50)
		if err != nil {
			return err
		}
		for _, e := range events {
			if e.Event.EntityID == marker {
				return repo.MarkFailed(txCtx, e.ID, "ปลายทางล่ม", retryAt)
			}
		}
		t.Fatal("ไม่พบ event ที่เพิ่งเขียน")
		return nil
	})
	if err != nil {
		t.Fatalf("MarkFailed ไม่สำเร็จ: %v", err)
	}

	var attempts int
	var lastErr string
	db.Raw("SELECT attempts, last_error FROM event_outbox WHERE entity_id = ?", marker).
		Row().Scan(&attempts, &lastErr)
	if attempts != 1 {
		t.Errorf("attempts = %d ต้องเป็น 1", attempts)
	}
	if lastErr != "ปลายทางล่ม" {
		t.Errorf("last_error = %q ต้องเก็บสาเหตุไว้", lastErr)
	}

	// ยังไม่ถึงเวลาลองใหม่ → ต้องไม่ถูกหยิบ
	err = tx.Within(ctx, func(txCtx context.Context) error {
		events, err := repo.ClaimPending(txCtx, 50)
		if err != nil {
			return err
		}
		for _, e := range events {
			if e.Event.EntityID == marker {
				t.Error("ยังไม่ถึง next_attempt_at ต้องไม่ถูกหยิบมาส่งซ้ำ")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ClaimPending ไม่สำเร็จ: %v", err)
	}
}

// TestOutboxRepository_TruncatesLongError
//
// ข้อความ error จาก database อาจยาวมาก ถ้าเก็บทั้งหมดแถวจะบวม
func TestOutboxRepository_TruncatesLongError(t *testing.T) {
	long := make([]byte, maxErrorLength+200)
	for i := range long {
		long[i] = 'x'
	}
	got := truncateError(string(long))
	if len(got) > maxErrorLength+len("…") {
		t.Errorf("ยาว %d ต้องถูกตัดที่ %d", len(got), maxErrorLength)
	}

	short := "สั้น"
	if truncateError(short) != short {
		t.Error("ข้อความสั้นต้องไม่ถูกแตะ")
	}
}
