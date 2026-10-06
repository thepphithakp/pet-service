package bootstrap

import (
	"github.com/gofiber/fiber/v2"

	"github.com/vertex/pet-service/pkg/middleware"
)

// registerRoutes รวมการประกาศ route ทั้งหมดไว้ที่เดียว
func registerRoutes(app *fiber.App, h handlers, auth middleware.AuthConfig, health *Health, chatServiceToken string) {
	// Public — ไม่ต้อง auth
	//
	// /livez  ไม่เช็ค dependency เลย — ถ้าเช็ค DB แล้ว DB ล่ม k8s จะฆ่าทุก pod
	//         พร้อมกันแล้ววนรีสตาร์ตไม่จบ ทำให้เหตุการณ์แย่ลง
	// /readyz เช็ค DB + สถานะปิดตัว — ล้มแล้ว pod แค่ถูกถอดออกจาก endpoints
	app.Get("/livez", health.Liveness)
	app.Get("/readyz", health.Readiness)

	// คงไว้เพื่อความเข้ากันได้กับ monitoring เดิมที่อาจเรียกอยู่
	app.Get("/health", health.Liveness)

	// /metrics ให้ Prometheus มาดึง
	//
	// ไม่ต้องใส่ auth เพราะ ingress route แค่ prefix /api/v1 เข้ามา
	// path นี้จึงเข้าถึงได้เฉพาะจากใน cluster เท่านั้น
	app.Get("/metrics", middleware.MetricsHandler())

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

	// Internal — service อื่นเรียก (ตอนนี้คือ chat-service) ไม่ใช่ผู้ใช้
	// ตรง ๆ จึงไม่มี JWT ให้ตรวจ ใช้ X-Service-Token แทน
	//
	// ⚠️ ต้องเป็น path ที่ไม่ขึ้นต้นด้วย "/api/v1" เด็ดขาด — group
	// app.Group("/api/v1", authMW) ด้านบนผูก middleware ด้วย string-prefix
	// match ของ Fiber ซึ่งครอบทุก path ที่ขึ้นต้นด้วย "/api/v1" ไม่ว่าจะ
	// ประกาศผ่าน group ไหนก็ตาม ถ้าเผลอใช้ "/api/v1/internal/..." ตรงนี้จะ
	// โดน JWT middleware ครอบไปด้วย ตอบ 401 "ต้องมี Authorization: Bearer"
	// แทนที่จะเช็ค X-Service-Token (เจอบั๊กนี้มาแล้วจริงตอนทำ
	// notification-service — ดู vertex-notification-service bootstrap/app.go)
	internal := app.Group("/internal/v1", middleware.NewServiceToken(chatServiceToken))
	h.internal.RegisterRoutes(internal)
}
