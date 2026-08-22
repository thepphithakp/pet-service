package middleware

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// TestMetrics_UsesRoutePatternNotRawPath กันปัญหา cardinality explosion
//
// ถ้าใช้ c.Path() เป็น label ทุก uuid จะกลายเป็น label ใหม่
// Prometheus จะเก็บ time series ไม่จำกัดจนหน่วยความจำหมด
func TestMetrics_UsesRoutePatternNotRawPath(t *testing.T) {
	app := fiber.New()
	app.Use(NewMetrics())
	app.Get("/pets/:id", func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/metrics", MetricsHandler())

	for _, id := range []string{"aaa", "bbb", "ccc"} {
		resp, err := app.Test(httptest.NewRequest("GET", "/pets/"+id, nil))
		if err != nil {
			t.Fatalf("ยิง request ไม่สำเร็จ: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d ต้องเป็น 200", resp.StatusCode)
		}
	}

	body := scrape(t, app)

	if strings.Contains(body, `route="/pets/aaa"`) {
		t.Error("metric ใช้ path ดิบเป็น label — จะทำให้ cardinality ระเบิด")
	}
	if !strings.Contains(body, `route="/pets/:id"`) {
		t.Errorf("ไม่พบ label route=\"/pets/:id\" ใน metric:\n%s", body)
	}
}

// TestMetrics_UnmatchedRouteCollapsed กันคนสแกน path มั่วแล้วทำให้ label แตก
func TestMetrics_UnmatchedRouteCollapsed(t *testing.T) {
	app := fiber.New()
	app.Use(NewMetrics())
	app.Get("/known", func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/metrics", MetricsHandler())

	for _, p := range []string{"/nope-1", "/nope-2"} {
		if _, err := app.Test(httptest.NewRequest("GET", p, nil)); err != nil {
			t.Fatalf("ยิง request ไม่สำเร็จ: %v", err)
		}
	}

	body := scrape(t, app)
	if strings.Contains(body, `route="/nope-1"`) {
		t.Error("path ที่ไม่ตรง route ไหนเลยถูกใช้เป็น label — ผู้ใช้ภายนอกทำให้ metric บวมได้")
	}
	if !strings.Contains(body, `route="unmatched"`) {
		t.Errorf("ไม่พบ label route=\"unmatched\":\n%s", body)
	}
}

// TestIsInfraPath ยืนยันว่า probe ไม่ถูกนับรวมกับ traffic ของผู้ใช้
func TestIsInfraPath(t *testing.T) {
	for _, p := range []string{"/livez", "/readyz", "/health", "/metrics"} {
		if !IsInfraPath(p) {
			t.Errorf("%s ควรเป็น infra path", p)
		}
	}
	if IsInfraPath("/api/v1/pets") {
		t.Error("/api/v1/pets ไม่ควรเป็น infra path")
	}
}

func scrape(t *testing.T, app *fiber.App) string {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", "/metrics", nil))
	if err != nil {
		t.Fatalf("ดึง /metrics ไม่สำเร็จ: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("/metrics status = %d ต้องเป็น 200", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("อ่าน body ไม่ได้: %v", err)
	}
	return string(b)
}
