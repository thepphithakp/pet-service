package main

import (
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

	// TODO(Phase 2.5): ลบออกเมื่อ Flyway migration job ทำงานแทนแล้ว
	if err := bootstrap.MigrateAndSeedLegacy(db); err != nil {
		log.Fatalf("AutoMigrate ล้มเหลว: %v", err)
	}

	publicKey, err := bootstrap.LoadPublicKey(cfg.JWT)
	if err != nil {
		log.Fatal(err)
	}

	app := bootstrap.NewApp(db, cfg, publicKey)

	log.Printf("pet-service กำลังรับ request ที่พอร์ต %s", cfg.Port)
	log.Fatal(app.Listen(":" + cfg.Port))
}
