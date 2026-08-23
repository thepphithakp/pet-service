//go:build integration

package bootstrap

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vertex/pet-service/internal/adapter/repository"
	"github.com/vertex/pet-service/internal/application"
	"github.com/vertex/pet-service/internal/config"
	"github.com/vertex/pet-service/internal/port"
	"github.com/vertex/pet-service/pkg/middleware"
)

// recordingSender บันทึกสิ่งที่ถูกส่ง และจำลองปลายทางล่มได้
type recordingSender struct {
	mu   sync.Mutex
	sent []string // idempotency key ที่ส่งสำเร็จ
	down bool
}

func (s *recordingSender) Send(_ context.Context, _ port.EventLog, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.down {
		return errors.New("event-service ล่ม")
	}
	s.sent = append(s.sent, key)
	return nil
}

func (s *recordingSender) keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.sent))
	copy(out, s.sent)
	return out
}

func (s *recordingSender) setDown(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.down = v
}

func outboxApp(t *testing.T, db *gorm.DB) (*fiber.App, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("สร้างคีย์ไม่สำเร็จ: %v", err)
	}
	app, _, _, _ := NewApp(db, config.Config{Port: "0"},
		middleware.AuthConfig{PublicKeys: []*rsa.PublicKey{&key.PublicKey}})
	return app, key
}

func pendingCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	db.Raw("SELECT count(*) FROM event_outbox WHERE published_at IS NULL").Scan(&n)
	return n
}

// TestOutbox_BusinessDataCommitsEvenWhenEventServiceIsDown
//
// Acceptance ข้อ 1 ของ Phase 7:
// kill event-service แล้วสร้าง log → ข้อมูลธุรกิจ commit สำเร็จ, event ค้างใน outbox
func TestOutbox_BusinessDataCommitsEvenWhenEventServiceIsDown(t *testing.T) {
	db := openTestDB(t)
	owner := uuid.New()
	app, key := outboxApp(t, db)

	petID := seedPet(t, db, owner)
	t.Cleanup(func() {
		db.Exec("DELETE FROM event_outbox WHERE entity_id = ?", petID.String())
		db.Exec("DELETE FROM water_logs WHERE pet_id = ?", petID)
		db.Exec("DELETE FROM pets WHERE id = ?", petID)
	})

	before := pendingCount(t, db)

	// ไม่ได้เริ่ม worker เลย = เหมือน event-service ล่มสนิท
	st, body := doJSONAs(t, app, "POST",
		fmt.Sprintf("/api/v1/pets/%s/water-logs", petID),
		fmt.Sprintf(`{"id":%q,"amount":42}`, uuid.New()), key, owner)
	if st != fiber.StatusCreated {
		t.Fatalf("status = %d ต้องเป็น 201 — ข้อมูลธุรกิจต้องบันทึกได้แม้ปลายทางล่ม (%s)", st, body)
	}

	var logs int64
	db.Raw("SELECT count(*) FROM water_logs WHERE pet_id = ?", petID).Scan(&logs)
	if logs != 1 {
		t.Fatalf("water_logs มี %d แถว ต้องมี 1", logs)
	}

	if got := pendingCount(t, db) - before; got != 1 {
		t.Fatalf("event ค้างใน outbox %d ตัว ต้องมี 1 — ถ้าเป็น 0 แปลว่า event หาย", got)
	}
}

// TestOutbox_RollbackTakesEventWithIt
//
// หัวใจของ outbox: ข้อมูลกับ event ต้องไปด้วยกันเสมอ
// ถ้าเขียนข้อมูลไม่สำเร็จ ต้องไม่มี event ค้างอยู่
func TestOutbox_RollbackTakesEventWithIt(t *testing.T) {
	db := openTestDB(t)
	owner := uuid.New()
	app, key := outboxApp(t, db)

	petID := seedPet(t, db, owner)
	t.Cleanup(func() {
		db.Exec("DELETE FROM event_outbox WHERE entity_id = ?", petID.String())
		db.Exec("DELETE FROM litter_logs WHERE pet_id = ?", petID)
		db.Exec("DELETE FROM pets WHERE id = ?", petID)
	})

	before := pendingCount(t, db)

	// type ที่ไม่มีใน master data → FK ล้ม → transaction ถูก rollback
	st, _ := doJSONAs(t, app, "POST",
		fmt.Sprintf("/api/v1/pets/%s/litter-logs", petID),
		`{"type":"ไม่มีชนิดนี้","amount":1}`, key, owner)
	if st < 400 {
		t.Fatalf("status = %d ต้องล้มเหลว", st)
	}

	var logs int64
	db.Raw("SELECT count(*) FROM litter_logs WHERE pet_id = ?", petID).Scan(&logs)
	if logs != 0 {
		t.Errorf("litter_logs มี %d แถว ต้องไม่มีเลย", logs)
	}
	if got := pendingCount(t, db) - before; got != 0 {
		t.Errorf("มี event ค้าง %d ตัวทั้งที่ข้อมูลถูก rollback — outbox ไม่ atomic", got)
	}
}

// TestOutbox_WorkerDeliversAfterEventServiceRecovers
//
// Acceptance ข้อ 2: เปิด event-service กลับมา → event ถูกส่งโดยไม่ต้อง restart pet-service
func TestOutbox_WorkerDeliversAfterEventServiceRecovers(t *testing.T) {
	db := openTestDB(t)
	owner := uuid.New()
	app, key := outboxApp(t, db)

	petID := seedPet(t, db, owner)
	t.Cleanup(func() {
		db.Exec("DELETE FROM event_outbox WHERE entity_id = ?", petID.String())
		db.Exec("DELETE FROM water_logs WHERE pet_id = ?", petID)
		db.Exec("DELETE FROM pets WHERE id = ?", petID)
	})

	st, _ := doJSONAs(t, app, "POST",
		fmt.Sprintf("/api/v1/pets/%s/water-logs", petID),
		fmt.Sprintf(`{"id":%q,"amount":10}`, uuid.New()), key, owner)
	if st != fiber.StatusCreated {
		t.Fatalf("status = %d", st)
	}

	sender := &recordingSender{down: true}
	worker := application.NewOutboxWorker(
		repository.NewGORMOutboxRepository(db),
		repository.NewGORMTxManager(db),
		sender,
		10*time.Millisecond,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go worker.Run(ctx)

	// ปลายทางล่ม — event ต้องยังค้างอยู่ ไม่หาย
	time.Sleep(150 * time.Millisecond)
	if len(sender.keys()) != 0 {
		t.Fatal("ปลายทางล่มแต่กลับส่งสำเร็จ")
	}

	// ปลายทางกลับมา — worker ต้องส่งเองโดยไม่ต้อง restart อะไร
	sender.setDown(false)

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if len(sender.keys()) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	keys := sender.keys()
	if len(keys) == 0 {
		t.Fatal("ปลายทางกลับมาแล้วแต่ event ไม่ถูกส่ง")
	}

	// ต้องถูก mark ว่าส่งแล้ว ไม่ส่งซ้ำเรื่อยๆ
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var unpublished int64
		db.Raw("SELECT count(*) FROM event_outbox WHERE entity_id = ? AND published_at IS NULL",
			petID.String()).Scan(&unpublished)
		if unpublished == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("ส่งสำเร็จแล้วแต่ไม่ได้ mark published — จะถูกส่งซ้ำไม่จบ")
}

// TestOutbox_IdempotencyKeyIsStableAcrossRetries
//
// retry ต้องใช้คีย์เดิม ไม่งั้น event-service จะมองว่าเป็นคนละตัวแล้วบันทึกซ้ำ
func TestOutbox_IdempotencyKeyIsStableAcrossRetries(t *testing.T) {
	db := openTestDB(t)
	owner := uuid.New()
	app, key := outboxApp(t, db)

	petID := seedPet(t, db, owner)
	t.Cleanup(func() {
		db.Exec("DELETE FROM event_outbox WHERE entity_id = ?", petID.String())
		db.Exec("DELETE FROM water_logs WHERE pet_id = ?", petID)
		db.Exec("DELETE FROM pets WHERE id = ?", petID)
	})

	st, _ := doJSONAs(t, app, "POST",
		fmt.Sprintf("/api/v1/pets/%s/water-logs", petID),
		fmt.Sprintf(`{"id":%q,"amount":10}`, uuid.New()), key, owner)
	if st != fiber.StatusCreated {
		t.Fatalf("status = %d", st)
	}

	repo := repository.NewGORMOutboxRepository(db)
	tx := repository.NewGORMTxManager(db)

	var first, second string
	for i, target := range []*string{&first, &second} {
		sender := &recordingSender{}
		w := application.NewOutboxWorker(repo, tx, sender, time.Second)

		// รอบแรกทำให้ล้มเพื่อให้ยังค้างอยู่ รอบสองปล่อยผ่าน
		sender.setDown(i == 0)
		_ = w
		err := tx.Within(context.Background(), func(txCtx context.Context) error {
			events, err := repo.ClaimPending(txCtx, 10)
			if err != nil {
				return err
			}
			for _, e := range events {
				if e.Event.EntityID == petID.String() {
					*target = e.IdempotencyKey
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("อ่าน outbox ไม่สำเร็จ: %v", err)
		}
	}

	if first == "" {
		t.Fatal("ไม่พบ event ใน outbox")
	}
	if first != second {
		t.Errorf("idempotency key เปลี่ยนระหว่าง retry: %q → %q", first, second)
	}
}
