package bootstrap

import (
	"crypto/rsa"

	"github.com/gofiber/fiber/v2"

	"github.com/vertex/pet-service/pkg/middleware"
)

// registerRoutes รวมการประกาศ route ทั้งหมดไว้ที่เดียว
// ทำให้เห็นภาพรวมได้ว่า endpoint ไหนอยู่หลัง auth และ endpoint ไหนเปิด
func registerRoutes(app *fiber.App, h handlers, publicKey *rsa.PublicKey) {
	// Public — ไม่ต้อง auth
	app.Get("/health", func(c *fiber.Ctx) error { return c.SendString("OK") })

	// Authenticated
	api := app.Group("/api/v1", middleware.NewAuthMiddleware(publicKey))
	h.pet.RegisterRoutes(api)
	h.caregiver.RegisterRoutes(api)
	h.litter.RegisterRoutes(api)
	h.water.RegisterRoutes(api)
	h.masterData.RegisterRoutes(api)
}
