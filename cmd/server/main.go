package main

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	// Application layer
	"github.com/vertex/pet-service/internal/application"

	// Adapters
	"github.com/vertex/pet-service/internal/adapter/handler"
	"github.com/vertex/pet-service/internal/adapter/repository"
	"github.com/vertex/pet-service/internal/adapter/event"

	// Domain
	"github.com/vertex/pet-service/internal/domain"

	// Shared packages
	"github.com/vertex/pet-service/pkg/middleware"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// --- Infrastructure: Database ---
	db := connectDB()
	migrateAndSeed(db)

	// --- Infrastructure: RSA Keys ---
	publicKey := loadPublicKey()

	// --- Dependency Injection (wire everything together) ---

	// Repositories (output adapters)
	petRepo := repository.NewGORMPetRepository(db)
	caregiverRepo := repository.NewGORMCaregiverRepository(db)
	litterRepo := repository.NewGORMLitterRepository(db)
	waterRepo := repository.NewGORMWaterRepository(db)
	
	// Event Publisher
	eventPublisher := event.NewHTTPEventPublisher()

	// Application services (use case implementations)
	petService := application.NewPetService(petRepo, eventPublisher)
	caregiverService := application.NewCaregiverService(caregiverRepo)
	litterService := application.NewLitterService(litterRepo, eventPublisher)
	waterService := application.NewWaterService(waterRepo, eventPublisher)
	masterDataService := application.NewMasterDataService()

	// HTTP Handlers (input adapters)
	petHandler := handler.NewPetHandler(petService)
	caregiverHandler := handler.NewCaregiverHandler(caregiverService)
	litterHandler := handler.NewLitterHandler(litterService)
	waterHandler := handler.NewWaterHandler(waterService)
	masterDataHandler := handler.NewMasterDataHandler(masterDataService)

	// --- Fiber App ---
	app := fiber.New(fiber.Config{
		BodyLimit:    50 * 1024 * 1024, // 50MB for image uploads
		ErrorHandler: middleware.ErrorHandler,
	})

	var maskRegex = regexp.MustCompile(`"(AvatarData|avatarData|avatar_data|token)":\s*"[^"]*"`)
	maskJSON := func(data []byte) string {
		if len(data) == 0 {
			return ""
		}
		return maskRegex.ReplaceAllString(string(data), `"$1": "[HIDDEN]"`)
	}

	app.Use(func(c *fiber.Ctx) error {
		start := time.Now()
		reqID := c.Get("X-Request-Id")
		if reqID == "" {
			reqID = uuid.New().String()
			c.Request().Header.Set("X-Request-Id", reqID)
		}
		c.Set("X-Request-Id", reqID)

		err := c.Next()

		latency := time.Since(start)
		reqBody := maskJSON(c.Body())
		resBody := maskJSON(c.Response().Body())

		logEntry := map[string]interface{}{
			"time":          time.Now().Format("2006-01-02T15:04:05.999Z07:00"),
			"level":         "info",
			"method":        c.Method(),
			"path":          c.Path(),
			"status":        c.Response().StatusCode(),
			"latency":       latency.String(),
			"request_id":    reqID,
			"user_id":       c.Locals("userId"),
			"source_system": c.Get("X-Source-System"),
			"device_id":     c.Get("X-Device-Id"),
			"ip":            c.IP(),
		}

		var reqObj interface{}
		if len(reqBody) > 0 {
			if e := json.Unmarshal([]byte(reqBody), &reqObj); e == nil {
				logEntry["req_body"] = reqObj
			} else {
				logEntry["req_body"] = reqBody
			}
		}

		var resObj interface{}
		if len(resBody) > 0 {
			if e := json.Unmarshal([]byte(resBody), &resObj); e == nil {
				logEntry["res_body"] = resObj
			} else {
				logEntry["res_body"] = resBody
			}
		}

		logJSON, _ := json.Marshal(logEntry)
		fmt.Println(string(logJSON))

		return err
	})

	// Health check (no auth)
	app.Get("/health", func(c *fiber.Ctx) error { return c.SendString("OK") })

	// Authenticated API routes
	api := app.Group("/api/v1", middleware.NewAuthMiddleware(publicKey))
	petHandler.RegisterRoutes(api)
	caregiverHandler.RegisterRoutes(api)
	litterHandler.RegisterRoutes(api)
	waterHandler.RegisterRoutes(api)
	masterDataHandler.RegisterRoutes(api)

	// --- Start ---
	port := os.Getenv("PORT")
	if port == "" {
		port = "4001"
	}
	log.Fatal(app.Listen(":" + port))
}

func connectDB() *gorm.DB {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=Asia/Bangkok",
		os.Getenv("DB_HOST"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"), os.Getenv("DB_PORT"),
	)

	var db *gorm.DB
	var err error
	for i := 0; i < 5; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		log.Printf("DB connection attempt %d failed: %v — retrying in 2s", i+1, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatal("Failed to connect to database after retries: ", err)
	}
	return db
}

func migrateAndSeed(db *gorm.DB) {
	if err := db.AutoMigrate(
		&repository.PetModel{},
		&repository.PermissionModel{},
		&repository.CaregiverModel{},
		&repository.LitterModel{},
		&repository.WaterModel{},
	); err != nil {
		log.Fatal("AutoMigrate failed: ", err)
	}

	// Seed master permissions
	permRepo := repository.NewGORMPermissionRepository(db)
	initialPermissions := []domain.PetPermission{
		{ID: "EDIT_PROFILE", Name: "Edit Profile", Description: "Can edit pet's basic profile details", IsActive: true},
		{ID: "MANAGE_MEDICAL", Name: "Manage Medical Records", Description: "Can view and add medical records/vaccines", IsActive: true},
		{ID: "MANAGE_WEIGHT", Name: "Update Weight Log", Description: "Can add weight records", IsActive: true},
		{ID: "MANAGE_TASKS", Name: "Manage Daily Tasks", Description: "Can view and tick off daily tasks", IsActive: true},
		{ID: "MANAGE_LITTER", Name: "Record Litter Box", Description: "Can record poop and pee events", IsActive: true},
	}
	if err := permRepo.Seed(nil, initialPermissions); err != nil {
		log.Println("Warning: failed to seed permissions:", err)
	}

	log.Println("Database migrated and seeded successfully.")
}

func loadPublicKey() *rsa.PublicKey {
	publicKeyBytes, err := os.ReadFile("keys/public.pem")
	if err != nil {
		log.Fatal("Failed to read public key: ", err)
	}
	key, err := jwt.ParseRSAPublicKeyFromPEM(publicKeyBytes)
	if err != nil {
		log.Fatal("Failed to parse public key: ", err)
	}
	return key
}
