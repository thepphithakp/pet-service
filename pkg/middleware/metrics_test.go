package middleware

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func newMetricsApp() *fiber.App {
	app := fiber.New()
	app.Use(NewMetrics())
	app.Get("/metrics", MetricsHandler())
	app.Get("/api/v1/pets/:id", func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Delete("/api/v1/pets/:id", func(c *fiber.Ctx) error { return c.SendStatus(204) })
	return app
}

func scrapeMetrics(t *testing.T, app *fiber.App) string {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", "/metrics", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("/metrics ตอบ %d ไม่ใช่ 200:\n%s", resp.StatusCode, string(b))
	}
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// regression ของบั๊กที่ทำให้ /metrics ตอบ 500 บน production มาตลอด
//
// Fiber คืน string ที่ชี้ไป buffer ของ request ซึ่งถูกใช้ซ้ำ พอ Prometheus
// เก็บไว้เป็น key ค่ามันเปลี่ยนตามทีหลัง label เลยกลายเป็น "GETETE"
// แล้วเกิด label ซ้ำจนทั้ง endpoint พัง
func TestMethodLabelIsNotCorruptedByBufferReuse(t *testing.T) {
	app := newMetricsApp()

	// สลับ method ไปมาหลายรอบเพื่อให้ buffer ถูกเขียนทับ
	for i := 0; i < 20; i++ {
		if _, err := app.Test(httptest.NewRequest("DELETE", "/api/v1/pets/abc", nil)); err != nil {
			t.Fatal(err)
		}
		if _, err := app.Test(httptest.NewRequest("GET", "/api/v1/pets/abc", nil)); err != nil {
			t.Fatal(err)
		}
	}

	// ถ้า label เพี้ยน /metrics จะตอบ 500 ตรงนี้เลย (scrapeMetrics เช็คให้แล้ว)
	body := scrapeMetrics(t, app)

	for _, want := range []string{`method="GET"`, `method="DELETE"`} {
		if !strings.Contains(body, want) {
			t.Errorf("ไม่พบ %s ใน /metrics", want)
		}
	}
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "http_requests_total{") {
			continue
		}
		if strings.Contains(line, `method="GETETE"`) || strings.Contains(line, `method="DELETEET"`) {
			t.Errorf("label ของ method เพี้ยน: %s", line)
		}
	}
}

// กับดักที่ทำให้ Prometheus ระเบิด: id ที่อยู่ใน path ทำให้ label แตกไม่จำกัด
func TestRouteLabelUsesPatternNotRawPath(t *testing.T) {
	app := newMetricsApp()
	for _, id := range []string{"aaa", "bbb", "ccc"} {
		if _, err := app.Test(httptest.NewRequest("GET", "/api/v1/pets/"+id, nil)); err != nil {
			t.Fatal(err)
		}
	}

	body := scrapeMetrics(t, app)
	if !strings.Contains(body, `route="/api/v1/pets/:id"`) {
		t.Errorf("ไม่พบ label ที่เป็น pattern ของ route")
	}
	for _, id := range []string{"aaa", "bbb", "ccc"} {
		if strings.Contains(body, "/api/v1/pets/"+id) {
			t.Errorf("id %q หลุดเข้าไปเป็น label — label จะแตกไม่จำกัด", id)
		}
	}
}

func TestUnmatchedPathsCollapse(t *testing.T) {
	app := newMetricsApp()
	for _, p := range []string{"/nope", "/also-nope"} {
		if _, err := app.Test(httptest.NewRequest("GET", p, nil)); err != nil {
			t.Fatal(err)
		}
	}

	body := scrapeMetrics(t, app)
	if !strings.Contains(body, `route="unmatched"`) {
		t.Error("path ที่ไม่ match ควรยุบเป็น unmatched")
	}
}

// /metrics ไม่ควรนับตัวเอง ไม่งั้นกราฟจะมี traffic พื้นหลังตลอดเวลา
func TestMetricsEndpointDoesNotCountItself(t *testing.T) {
	app := newMetricsApp()
	scrapeMetrics(t, app)
	body := scrapeMetrics(t, app)

	if strings.Contains(body, `route="/metrics"`) {
		t.Error("นับ /metrics ของตัวเองด้วย")
	}
}
