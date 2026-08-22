package main

import (
	"context"
	"log"

	"github.com/vertex/pet-service/internal/bootstrap"
	"github.com/vertex/pet-service/internal/config"
	"github.com/vertex/pet-service/pkg/middleware"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("ตั้งค่าไม่ถูกต้อง: %v", err)
	}

	middleware.SetupLogger(cfg.Log.Level)
	if cfg.Log.Body {
		log.Println("⚠️  LOG_BODY=true — request/response body จะถูกเขียนลง log ห้ามเปิดบน production")
	}

	db, err := bootstrap.NewDB(cfg.DB)
	if err != nil {
		log.Fatal(err)
	}

	// schema จัดการโดย Flyway แล้ว ไม่ใช่ AutoMigrate
	// ตรงนี้แค่ยืนยันว่า migration รันครบก่อนรับ request
	if err := bootstrap.AssertSchemaVersion(context.Background(), db); err != nil {
		log.Fatal(err)
	}

	auth, err := bootstrap.NewAuthConfig(cfg.JWT)
	if err != nil {
		log.Fatal(err)
	}

	app := bootstrap.NewApp(db, cfg, auth)

	log.Printf("pet-service กำลังรับ request ที่พอร์ต %s", cfg.Port)
	log.Fatal(app.Listen(":" + cfg.Port))
}
