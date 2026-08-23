//go:build integration

package bootstrap

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/vertex/pet-service/internal/config"
	"github.com/vertex/pet-service/pkg/middleware"
)

// TestGracefulShutdown_NoDroppedRequests พิสูจน์ว่า request ที่กำลังทำงานอยู่
// ตอนได้รับ SIGTERM จะทำงานจนจบ ไม่ถูกตัดทิ้ง
//
// เป็นข้อพิสูจน์หลักของ Phase 6 — ก่อนหน้านี้ app.Listen ถูกเรียกใน log.Fatal
// พอ pod โดน SIGTERM process ตายทันที request ที่ค้างอยู่หายหมด
func TestGracefulShutdown_NoDroppedRequests(t *testing.T) {
	db := openTestDB(t)
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	app, health, publisher, _ := NewApp(db, config.Config{Port: "0"}, middleware.AuthConfig{
		PublicKeys: []*rsa.PublicKey{&key.PublicKey},
	})

	// endpoint ที่ทำงานนาน จำลอง request ที่ยังไม่เสร็จตอนสั่งปิด
	var inFlight, completed atomic.Int32
	app.Get("/slow", func(c *fiber.Ctx) error {
		inFlight.Add(1)
		defer inFlight.Add(-1)
		time.Sleep(700 * time.Millisecond)
		completed.Add(1)
		return c.SendString("done")
	})

	const n = 5
	var wg sync.WaitGroup
	results := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			res, err := app.Test(httptest.NewRequest("GET", "/slow", nil), 10000)
			if err != nil {
				results[idx] = -1
				return
			}
			results[idx] = res.StatusCode
		}(i)
	}

	// รอให้ request เข้าไปทำงานจริงก่อนสั่งปิด
	for i := 0; i < 100 && inFlight.Load() == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if inFlight.Load() == 0 {
		t.Fatal("ไม่มี request กำลังทำงาน — test ตั้งไม่ถูก")
	}

	// จำลองลำดับเดียวกับตอนได้รับ SIGTERM
	health.BeginShutdown()

	// readiness ต้องตอบ 503 ทันที เพื่อให้ k8s ถอด pod ออกจาก endpoints
	res, err := app.Test(httptest.NewRequest("GET", "/readyz", nil), 3000)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 503 {
		t.Fatalf("readyz ระหว่างปิด = %d ต้องการ 503", res.StatusCode)
	}

	// liveness ต้องยัง 200 ไม่งั้น k8s จะ SIGKILL ทิ้งกลางคัน
	res, _ = app.Test(httptest.NewRequest("GET", "/livez", nil), 3000)
	if res.StatusCode != 200 {
		t.Fatalf("livez ระหว่างปิด = %d ต้องยัง 200", res.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	publisher.Drain(ctx)

	wg.Wait()

	for i, code := range results {
		if code != 200 {
			t.Errorf("request %d ได้ %d — ต้องทำงานจนจบแม้สั่งปิดระหว่างทาง", i, code)
		}
	}
	if got := completed.Load(); got != n {
		t.Fatalf("ทำงานจนจบ %d จาก %d request", got, n)
	}
	t.Logf("✅ request ทั้ง %d ตัวทำงานจนจบหลังสั่งปิด", n)
}

// Drain ต้องไม่ค้างตลอดกาลถ้า event ส่งไม่เสร็จ
func TestPublisherDrain_RespectsTimeout(t *testing.T) {
	db := openTestDB(t)
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, _, publisher, _ := NewApp(db, config.Config{Port: "0"}, middleware.AuthConfig{
		PublicKeys: []*rsa.PublicKey{&key.PublicKey},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	publisher.Drain(ctx)
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("Drain ใช้เวลา %v — ต้องเคารพ timeout ของ context", d)
	}
}
