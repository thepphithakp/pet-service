package bootstrap

import (
	"github.com/gofiber/fiber/v2"

	"github.com/vertex/pet-service/pkg/middleware"
)

// registerRoutes รวมการประกาศ route ทั้งหมดไว้ที่เดียว
func registerRoutes(app *fiber.App, h handlers, auth middleware.AuthConfig, health *Health) {
	// Public — ไม่ต้อง auth
	//
	// /livez  ไม่เช็ค dependency เลย — ถ้าเช็ค DB แล้ว DB ล่ม k8s จะฆ่าทุก pod
	//         พร้อมกันแล้ววนรีสตาร์ตไม่จบ ทำให้เหตุการณ์แย่ลง
	// /readyz เช็ค DB + สถานะปิดตัว — ล้มแล้ว pod แค่ถูกถอดออกจาก endpoints
	app.Get("/livez", health.Liveness)
	app.Get("/readyz", health.Readiness)

	// คงไว้เพื่อความเข้ากันได้กับ monitoring เดิมที่อาจเรียกอยู่
	app.Get("/health", health.Liveness)

	authMW := middleware.NewAuthMiddleware(auth)

	// ผู้ใช้ทั่วไป
	api := app.Group("/api/v1", authMW)
	h.pet.RegisterRoutes(api)
	h.caregiver.RegisterRoutes(api)
	h.litter.RegisterRoutes(api)
	h.water.RegisterRoutes(api)
	h.masterData.RegisterRoutes(api)

	// Admin — แยก group ให้เห็นชัด
	//
	// ⚠️ การตรวจ capability จริงอยู่ที่ชั้น service ไม่ใช่ที่ group นี้
	//    group แยกไว้เพื่อความชัดเจนและเผื่อใส่ middleware เพิ่มในอนาคตเท่านั้น
	//    ห้ามพึ่ง group เป็นด่านเดียว
	admin := app.Group("/api/v1/admin", authMW)
	h.pet.RegisterAdminRoutes(admin)
	h.masterData.RegisterAdminRoutes(admin)
}
