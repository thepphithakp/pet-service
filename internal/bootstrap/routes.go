package bootstrap

import (
	"github.com/gofiber/fiber/v2"

	"github.com/vertex/pet-service/pkg/middleware"
)

// registerRoutes รวมการประกาศ route ทั้งหมดไว้ที่เดียว
func registerRoutes(app *fiber.App, h handlers, auth middleware.AuthConfig) {
	// Public — ไม่ต้อง auth
	app.Get("/health", func(c *fiber.Ctx) error { return c.SendString("OK") })

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
}
