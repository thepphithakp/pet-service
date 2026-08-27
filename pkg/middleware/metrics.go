package middleware

import (
	"errors"
	"strconv"

	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ชุด metric ของ HTTP layer
//
// ใช้ c.Route().Path เป็น label ไม่ใช่ c.Path() เพราะ path จริงมี id ปนอยู่
// (/api/v1/pets/<uuid>/litter-logs) ถ้าใช้ path ดิบ label จะแตกไม่จำกัด
// ทำให้ Prometheus ระเบิด (cardinality explosion)
var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "จำนวน HTTP request ทั้งหมด แยกตาม method / route / status",
		},
		[]string{"method", "route", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "เวลาที่ใช้ต่อ request",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)

	httpRequestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "จำนวน request ที่กำลังทำงานอยู่",
		},
	)

	// outboxPending คือจำนวน event ที่ยังส่งไม่สำเร็จ
	//
	// ค่านี้ควรวนกลับมาใกล้ 0 เสมอ ถ้าค้างสูงต่อเนื่องแปลว่า event-service
	// มีปัญหาหรือ worker ไม่ทำงาน — ตั้ง alert จากค่านี้ได้เลย
	outboxPending = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "outbox_pending_count",
			Help: "จำนวน event ใน outbox ที่ยังส่งไม่สำเร็จ",
		},
	)

	// outboxDeliveries นับผลการส่งแยกตามผลลัพธ์
	outboxDeliveries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "outbox_deliveries_total",
			Help: "จำนวนครั้งที่พยายามส่ง event แยกตามผลลัพธ์",
		},
		[]string{"result"},
	)
)

// SetOutboxPending อัปเดตจำนวน event ที่ค้างอยู่
func SetOutboxPending(n int64) { outboxPending.Set(float64(n)) }

// CountOutboxDelivery นับผลการส่งหนึ่งครั้ง — result เป็น "success" หรือ "failure"
func CountOutboxDelivery(result string) { outboxDeliveries.WithLabelValues(result).Inc() }

func init() {
	prometheus.MustRegister(httpRequestsTotal, httpRequestDuration, httpRequestsInFlight,
		outboxPending, outboxDeliveries)
}

// NewMetrics คืน middleware ที่เก็บ metric ของทุก request
func NewMetrics() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// ไม่นับตัวเอง — Prometheus ยิงทุก 30 วินาที ถ้านับด้วย
		// กราฟจะมี traffic พื้นหลังตลอดเวลาทั้งที่ไม่มีใครใช้งานจริง
		if c.Path() == "/metrics" {
			return c.Next()
		}

		// ⚠️ ต้องคัดลอก string ที่ได้จาก c.Method() ก่อนเสมอ
		//
		// Fiber คืน string ที่ชี้ไปยัง buffer ของ request ซึ่งถูกใช้ซ้ำกับ
		// request ถัดไป ส่วน Prometheus เก็บ string นั้นไว้เป็น key ใน map
		// ตลอดอายุของ process — พอ buffer ถูกเขียนทับ key ที่เก็บไว้ก็เปลี่ยนตาม
		//
		// เกิดขึ้นจริงบน production: label กลายเป็น "GETETE" (GET ทับด้วยเศษ
		// ของ DELETE) แล้วเกิด label ซ้ำจน /metrics ตอบ 500 ทั้ง endpoint
		// ผลคือ Prometheus scrape ไม่ผ่านเลยแม้แต่ครั้งเดียว โดยที่ ServiceMonitor
		// ยังเขียวอยู่และไม่มีอะไรฟ้อง
		method := utils.CopyString(c.Method())

		httpRequestsInFlight.Inc()
		defer httpRequestsInFlight.Dec()

		timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
			httpRequestDuration.WithLabelValues(method, routeLabel(c)).Observe(v)
		}))

		err := c.Next()

		// อ่าน status หลัง c.Next() เพราะ error handler อาจเปลี่ยน status
		status := c.Response().StatusCode()
		if err != nil {
			var fe *fiber.Error
			if errors.As(err, &fe) {
				status = fe.Code
			}
		}
		httpRequestsTotal.WithLabelValues(method, routeLabel(c), strconv.Itoa(status)).Inc()
		timer.ObserveDuration()

		return err
	}
}

// routeLabel คืน pattern ของ route ที่จับคู่ได้ ไม่ใช่ path จริง
// ถ้าไม่ตรง route ไหนเลย (404) ยุบเป็น "unmatched" เพื่อไม่ให้ label แตก
func routeLabel(c *fiber.Ctx) string {
	r := c.Route()
	if r == nil || r.Path == "" {
		return "unmatched"
	}
	// request ที่ไม่ตรง route ไหนเลย fiber จะคืน route ของ app.Use ซึ่ง Path = "/"
	// ถ้า pattern เป็น "/" แต่ path จริงไม่ใช่ "/" แปลว่าไม่ได้ match จริง
	if r.Path == "/" && c.Path() != "/" {
		return "unmatched"
	}
	return r.Path
}

// MetricsHandler คือ endpoint /metrics ให้ Prometheus มาดึง
func MetricsHandler() fiber.Handler {
	return adaptor.HTTPHandler(promhttp.Handler())
}
