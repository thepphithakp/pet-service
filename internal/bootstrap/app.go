// Package bootstrap ประกอบ dependency ทั้งหมดเข้าด้วยกัน
// แก้ A-4: เดิม cmd/server/main.go ยาว 213 บรรทัดและปนทั้ง DI, migration,
// seed, logging middleware, การอ่าน key และ config
package bootstrap

import (
	"crypto/rsa"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/vertex/pet-service/internal/adapter/event"
	"github.com/vertex/pet-service/internal/adapter/handler"
	"github.com/vertex/pet-service/internal/adapter/repository"
	"github.com/vertex/pet-service/internal/application"
	"github.com/vertex/pet-service/internal/config"
	"github.com/vertex/pet-service/pkg/middleware"
)

// bodyLimit รองรับการอัปโหลดรูป avatar
// TODO(Phase 5.1): ลดลงได้เมื่อ avatar ย้ายออกจาก request body
const bodyLimit = 50 * 1024 * 1024

// handlers รวม input adapter ทั้งหมดที่ต้องลงทะเบียน route
type handlers struct {
	pet        *handler.PetHandler
	caregiver  *handler.CaregiverHandler
	litter     *handler.LitterHandler
	water      *handler.WaterHandler
	masterData *handler.MasterDataHandler
}

// wire ประกอบ repository → service → handler
func wire(db *gorm.DB, cfg config.Config) handlers {
	// Output adapters
	petRepo := repository.NewGORMPetRepository(db)
	caregiverRepo := repository.NewGORMCaregiverRepository(db)
	litterRepo := repository.NewGORMLitterRepository(db)
	waterRepo := repository.NewGORMWaterRepository(db)
	eventPublisher := event.NewHTTPEventPublisher(cfg.EventServiceURL)

	// Use cases
	petService := application.NewPetService(petRepo, eventPublisher)
	caregiverService := application.NewCaregiverService(caregiverRepo)
	litterService := application.NewLitterService(litterRepo, eventPublisher)
	waterService := application.NewWaterService(waterRepo, eventPublisher)
	masterDataService := application.NewMasterDataService()

	// Input adapters
	return handlers{
		pet:        handler.NewPetHandler(petService),
		caregiver:  handler.NewCaregiverHandler(caregiverService),
		litter:     handler.NewLitterHandler(litterService),
		water:      handler.NewWaterHandler(waterService),
		masterData: handler.NewMasterDataHandler(masterDataService),
	}
}

// NewApp สร้าง fiber app ที่พร้อมรับ request
func NewApp(db *gorm.DB, cfg config.Config, publicKey *rsa.PublicKey) *fiber.App {
	app := fiber.New(fiber.Config{
		BodyLimit:    bodyLimit,
		ErrorHandler: middleware.ErrorHandler,
	})

	app.Use(middleware.NewRequestID())
	app.Use(middleware.NewAccessLog())

	registerRoutes(app, wire(db, cfg), publicKey)
	return app
}
