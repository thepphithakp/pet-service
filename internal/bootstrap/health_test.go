package bootstrap

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func healthApp(h *Health) *fiber.App {
	app := fiber.New()
	app.Get("/livez", h.Liveness)
	app.Get("/readyz", h.Readiness)
	return app
}

func status(t *testing.T, app *fiber.App, path string) (int, string) {
	t.Helper()
	res, err := app.Test(httptest.NewRequest("GET", path, nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	s, _ := m["status"].(string)
	return res.StatusCode, s
}

// TestLiveness_IgnoresDependencies คือกฎที่สำคัญที่สุดของ liveness
//
// ถ้า liveness เช็ค DB แล้ว DB ล่ม k8s จะฆ่า pod ทุกตัวพร้อมกัน
// แล้ววนรีสตาร์ตไม่จบ ทำให้เหตุการณ์แย่ลงแทนที่จะดีขึ้น
//
// ที่นี่ส่ง db = nil เข้าไปเลย — liveness ต้องยังตอบ 200
func TestLiveness_IgnoresDependencies(t *testing.T) {
	h := NewHealth(nil)
	code, st := status(t, healthApp(h), "/livez")
	if code != 200 || st != "ok" {
		t.Fatalf("liveness = %d %q — ต้องไม่สนใจ dependency ใดๆ", code, st)
	}
}

// readiness ต้องตอบ 503 ทันทีที่เริ่มปิดตัว เพื่อให้ k8s ถอด pod ออกจาก endpoints
func TestReadiness_ShuttingDown(t *testing.T) {
	h := NewHealth(nil)
	h.BeginShutdown()

	code, st := status(t, healthApp(h), "/readyz")
	if code != 503 || st != "shutting_down" {
		t.Fatalf("readiness = %d %q ต้องการ 503 shutting_down", code, st)
	}

	// liveness ต้องยัง 200 ระหว่างปิดตัว
	// ไม่งั้น k8s จะ SIGKILL ทิ้งกลางคัน แทนที่จะรอให้ปิดเรียบร้อย
	if code, _ := status(t, healthApp(h), "/livez"); code != 200 {
		t.Fatalf("liveness ระหว่างปิดตัว = %d ต้องยังเป็น 200", code)
	}
}

// DB ต่อไม่ได้ → readiness 503 (แต่ไม่ panic)
func TestReadiness_DBUnavailable(t *testing.T) {
	h := NewHealth(nil)
	if code, _ := status(t, healthApp(h), "/readyz"); code != 503 {
		t.Fatalf("readiness = %d ต้องการ 503 เมื่อ DB ใช้ไม่ได้", code)
	}
}

func TestBeginShutdown_Idempotent(t *testing.T) {
	h := NewHealth(nil)
	h.BeginShutdown()
	h.BeginShutdown()
	if code, _ := status(t, healthApp(h), "/readyz"); code != 503 {
		t.Fatal("เรียกซ้ำต้องไม่เปลี่ยนพฤติกรรม")
	}
}
