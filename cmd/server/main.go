package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/vertex/pet-service/internal/bootstrap"
	"github.com/vertex/pet-service/internal/config"
	"github.com/vertex/pet-service/pkg/middleware"
)

// fatal จบ process พร้อม log ที่เป็น JSON เหมือน log line อื่น
//
// log.Fatal ของ stdlib ผ่าน bridge ของ slog ก็จริง แต่ออกมาเป็น level INFO เสมอ
// ทำให้ตอนไล่ปัญหา กรอง level=ERROR แล้วไม่เจอสาเหตุที่ทำให้ pod ตาย
func fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

func main() {
	// ตั้ง JSON logger ก่อนอ่าน config เพื่อให้ error ตอนอ่าน config เป็น JSON ด้วย
	// เดี๋ยวตั้งซ้ำอีกครั้งด้วย level จริงจาก config
	middleware.SetupLogger("")

	cfg, err := config.Load()
	if err != nil {
		fatal("ตั้งค่าไม่ถูกต้อง", "error", err)
	}

	middleware.SetupLogger(cfg.Log.Level)
	if cfg.Log.Body {
		slog.Warn("LOG_BODY=true — request/response body จะถูกเขียนลง log ห้ามเปิดบน production")
	}

	db, err := bootstrap.NewDB(cfg.DB)
	if err != nil {
		fatal("เชื่อมต่อฐานข้อมูลไม่สำเร็จ", "error", err)
	}

	// schema จัดการโดย Flyway แล้ว ไม่ใช่ AutoMigrate
	// ตรงนี้แค่ยืนยันว่า migration รันครบก่อนรับ request
	if err := bootstrap.AssertSchemaVersion(context.Background(), db); err != nil {
		fatal("schema ยังไม่พร้อม (Flyway migration รันครบหรือยัง)", "error", err)
	}

	auth, err := bootstrap.NewAuthConfig(cfg.JWT)
	if err != nil {
		fatal("ตั้งค่า JWT ไม่สำเร็จ", "error", err)
	}

	app, health, publisher := bootstrap.NewApp(db, cfg, auth)

	// รับ signal ก่อนเริ่ม listen เพื่อไม่ให้พลาด SIGTERM ที่มาเร็ว
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("pet-service กำลังรับ request", "port", cfg.Port)
		if err := app.Listen(":" + cfg.Port); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen ล้มเหลว", "error", err)
			stop <- syscall.SIGTERM
		}
	}()

	<-stop
	shutdown(app, health, publisher, db, cfg.Shutdown)
}

// shutdown ปิดตัวแบบไม่ทิ้ง request ที่ค้างอยู่
//
// ลำดับสำคัญมาก — ถ้าเรียงผิดจะยัง drop request อยู่ดี
//
//  1. ปิด readiness ก่อน  → k8s ถอด pod ออกจาก Service endpoints
//  2. รอให้ endpoints กระจายไปทุก node
//  3. ค่อยหยุดรับ connection ใหม่ แล้วรอ request ที่ค้างอยู่จนจบ
//
// ขั้นที่ 2 คือขั้นที่คนข้ามบ่อยที่สุด
// การถอด endpoint ของ k8s เป็น asynchronous — kube-proxy/ingress บาง node
// ยังส่ง request มาอีกหลายวินาทีหลัง pod เข้าสถานะ Terminating
// ถ้าปิด listener ทันทีที่ได้ SIGTERM request เหล่านั้นจะโดน connection refused
func shutdown(
	app *fiber.App,
	health *bootstrap.Health,
	publisher interface{ Drain(context.Context) },
	db *gorm.DB,
	cfg config.ShutdownConfig,
) {
	slog.Info("ได้รับสัญญาณปิด เริ่มปิดตัวแบบ graceful")

	health.BeginShutdown()
	slog.Info("ปิด readiness แล้ว รอให้ k8s ถอด endpoint", "wait", cfg.DrainDelay)
	time.Sleep(cfg.DrainDelay)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	if err := app.ShutdownWithContext(ctx); err != nil {
		slog.Error("ปิด HTTP server ไม่เรียบร้อย", "error", err)
	} else {
		slog.Info("request ที่ค้างอยู่ทำงานจนจบแล้ว")
	}

	// event ที่ยังส่งไม่เสร็จ ให้โอกาสทำงานต่อจนจบ
	// ไม่งั้น event ที่เพิ่งถูกสร้างจาก request สุดท้ายจะหายไปเฉยๆ
	publisher.Drain(ctx)

	if err := bootstrap.CloseDB(db); err != nil {
		slog.Error("ปิด connection ฐานข้อมูลไม่เรียบร้อย", "error", err)
	}
	slog.Info("ปิดตัวเรียบร้อย")
}
