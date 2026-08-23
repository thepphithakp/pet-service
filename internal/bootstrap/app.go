// Package bootstrap ประกอบ dependency ทั้งหมดเข้าด้วยกัน
// แก้ A-4: เดิม cmd/server/main.go ยาว 213 บรรทัดและปนทั้ง DI, migration,
// seed, logging middleware, การอ่าน key และ config
package bootstrap

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"
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
// deps คือสิ่งที่ผู้เรียกต้องใช้ต่อหลังสร้าง app เสร็จ
type handlers struct {
	pet        *handler.PetHandler
	caregiver  *handler.CaregiverHandler
	litter     *handler.LitterHandler
	water      *handler.WaterHandler
	masterData *handler.MasterDataHandler
}

// wire ประกอบ repository → service → handler
func wire(db *gorm.DB, cfg config.Config) (handlers, *event.HTTPEventPublisher) {
	// Output adapters
	petRepo := repository.NewGORMPetRepository(db)
	caregiverRepo := repository.NewGORMCaregiverRepository(db)
	permissionRepo := repository.NewGORMPermissionRepository(db)
	litterRepo := repository.NewGORMLitterRepository(db)
	waterRepo := repository.NewGORMWaterRepository(db)
	capabilityRepo := repository.NewGORMCapabilityRepository(db)
	masterDataRepo := repository.NewGORMMasterDataRepository(db)
	eventPublisher := event.NewHTTPEventPublisher(cfg.EventServiceURL, cfg.EventIngestToken)

	// Authorizer ใช้ร่วมกันทุก service — บังคับสิทธิ์ที่ชั้น application
	authz := application.NewAuthorizer(petRepo, capabilityRepo)

	// Use cases
	petService := application.NewPetService(petRepo, eventPublisher, authz)
	caregiverService := application.NewCaregiverService(caregiverRepo, permissionRepo, authz)
	litterService := application.NewLitterService(litterRepo, eventPublisher, authz)
	waterService := application.NewWaterService(waterRepo, eventPublisher, authz)
	masterDataService := application.NewMasterDataService(masterDataRepo, permissionRepo, authz)

	// Input adapters
	return handlers{
		pet:        handler.NewPetHandler(petService, cfg.PetListIncludeAvatar),
		caregiver:  handler.NewCaregiverHandler(caregiverService),
		litter:     handler.NewLitterHandler(litterService),
		water:      handler.NewWaterHandler(waterService),
		masterData: handler.NewMasterDataHandler(masterDataService, masterDataService),
	}, eventPublisher
}

// NewApp สร้าง fiber app ที่พร้อมรับ request
func NewApp(db *gorm.DB, cfg config.Config, auth middleware.AuthConfig) (*fiber.App, *Health, *event.HTTPEventPublisher) {
	app := fiber.New(fiber.Config{
		BodyLimit:    bodyLimit,
		ErrorHandler: middleware.ErrorHandler,
		// ปิดกล่อง ASCII ตอน start เพราะเป็นบรรทัดเดียวใน log ที่ parse
		// เป็น JSON ไม่ได้ ทำให้ระบบเก็บ log ต้องมีข้อยกเว้น
		// ข้อมูลที่จำเป็น (พอร์ต) มี log ของเราเองอยู่แล้ว
		DisableStartupMessage: true,
	})

	// recover ต้องมาก่อนทุกอย่าง — เดิมไม่มีเลย panic ใน handler ทำให้
	// middleware ที่เหลือไม่ทำงานต่อและ client ได้ connection ที่ตายไปเฉยๆ (S-10)
	app.Use(recover.New())
	app.Use(middleware.NewRequestID())
	// metrics มาก่อน limiter เพื่อให้นับ request ที่โดน 429 ด้วย
	app.Use(middleware.NewMetrics())
	app.Use(middleware.NewAccessLog(middleware.LogConfig{LogBody: cfg.Log.Body}))
	app.Use(limiter.New(limiter.Config{
		Max:        300,
		Expiration: time.Minute,
		// probe ของ k8s กับ scrape ของ Prometheus ยิงถี่และมาจาก IP เดียว
		// ถ้านับรวมด้วย พอ traffic สูงๆ probe จะโดน 429 แล้ว k8s ฆ่า pod ทิ้ง
		Next: func(c *fiber.Ctx) bool {
			return middleware.IsInfraPath(c.Path())
		},
		KeyGenerator: func(c *fiber.Ctx) string {
			if uid, ok := c.Locals("userId").(string); ok && uid != "" {
				return uid
			}
			return c.IP()
		},
	}))

	h, publisher := wire(db, cfg)
	health := NewHealth(db)
	registerRoutes(app, h, auth, health)
	return app, health, publisher
}
