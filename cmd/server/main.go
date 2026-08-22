package main

import (
	"context"
	"log"

	"github.com/vertex/pet-service/internal/bootstrap"
	"github.com/vertex/pet-service/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("ตั้งค่าไม่ถูกต้อง: %v", err)
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

	publicKey, err := bootstrap.LoadPublicKey(cfg.JWT)
	if err != nil {
		log.Fatal(err)
	}

	app := bootstrap.NewApp(db, cfg, publicKey)

	log.Printf("pet-service กำลังรับ request ที่พอร์ต %s", cfg.Port)
	log.Fatal(app.Listen(":" + cfg.Port))
}
