package bootstrap

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// dbPingTimeout — probe ยิงทุกไม่กี่วินาที ถ้าไม่ใส่ timeout แล้ว DB ค้าง
// goroutine ของ probe จะกองสะสมจนกินหน่วยความจำ
const dbPingTimeout = 2 * time.Second

// Health รวมสถานะที่ probe ของ Kubernetes ต้องใช้
type Health struct {
	db *gorm.DB

	// shuttingDown ถูกตั้งเป็น true ทันทีที่ได้รับ SIGTERM
	//
	// ทำให้ /readyz ตอบ 503 ก่อนที่จะเริ่มปิดจริง
	// k8s จะถอด pod ออกจาก Service endpoints แล้วหยุดส่ง request ใหม่มา
	// ระหว่างนั้น request ที่ค้างอยู่ยังทำงานต่อจนจบ
	shuttingDown atomic.Bool
}

func NewHealth(db *gorm.DB) *Health { return &Health{db: db} }

// BeginShutdown ทำให้ readiness เริ่มตอบ 503
func (h *Health) BeginShutdown() { h.shuttingDown.Store(true) }

// Liveness ตอบว่า process ยังทำงานอยู่ไหม
//
// ⚠️ จงใจไม่เช็ค DB
//
// ถ้า liveness เช็ค DB แล้ว DB ล่ม k8s จะฆ่า pod ทุกตัวพร้อมกันแล้ววนรีสตาร์ตไม่จบ
// ซึ่งทำให้เหตุการณ์แย่ลงแทนที่จะดีขึ้น — pod ที่ต่อ DB ไม่ได้ควรถูกถอดออกจาก
// endpoints (readiness) ไม่ใช่ถูกฆ่า
func (h *Health) Liveness(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}

// Readiness ตอบว่าพร้อมรับ request จริงไหม
//
// เช็คว่า DB ต่อได้ และไม่ได้อยู่ระหว่างปิดตัว
func (h *Health) Readiness(c *fiber.Ctx) error {
	if h.shuttingDown.Load() {
		return c.Status(fiber.StatusServiceUnavailable).
			JSON(fiber.Map{"status": "shutting_down"})
	}

	// readiness ต้องไม่ panic ไม่ว่ากรณีไหน
	// probe ที่ panic ทำให้ k8s ตีความสถานะผิดและ debug ยาก
	if h.db == nil {
		return c.Status(fiber.StatusServiceUnavailable).
			JSON(fiber.Map{"status": "db_unavailable", "error": "ยังไม่ได้ตั้งค่าฐานข้อมูล"})
	}

	ctx, cancel := context.WithTimeout(c.UserContext(), dbPingTimeout)
	defer cancel()

	sqlDB, err := h.db.DB()
	if err == nil {
		err = sqlDB.PingContext(ctx)
	}
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).
			JSON(fiber.Map{"status": "db_unavailable", "error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}
