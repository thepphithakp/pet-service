package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/config"
	"github.com/vertex/pet-service/pkg/middleware"
)

// TestInternalRoutes_DoNotCrossAuthenticate ป้องกันไม่ให้ bug นี้กลับมาอีก:
//
// app.Group("/api/v1", authMW) ผูก middleware ด้วย string-prefix match ของ
// Fiber ไม่ใช่ตาม path segment ถ้า /internal/v1/... เผลอถูกตั้งเป็น
// /api/v1/internal/... จะโดน JWT middleware ของ group ด้านบนครอบไปด้วย
// ตอบ 401 "ต้องมี Authorization: Bearer" แทนที่จะเช็ค X-Service-Token —
// เจอบั๊กแบบเดียวกันนี้มาแล้วจริงตอนทำ notification-service และ chat-service
func TestInternalRoutes_DoNotCrossAuthenticate(t *testing.T) {
	pemStr, _ := publicKeyPEM(t)
	authCfg, err := NewAuthConfig(config.JWTConfig{PublicKeysPEM: pemStr})
	if err != nil {
		t.Fatalf("สร้าง auth config ไม่สำเร็จ: %v", err)
	}

	const serviceToken = "test-chat-service-token-at-least-32-chars"
	app, _, _, _ := NewApp(nil, config.Config{ChatServiceToken: serviceToken}, authCfg)

	t.Run("/api/v1/pets ปฏิเสธ request ที่ไม่มี JWT", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pets", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test ล้มเหลว: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("ต้องการ 401 ได้ %d", resp.StatusCode)
		}
	})

	t.Run("/internal/v1/pets/:id/link ปฏิเสธ request ที่ไม่มี service token", func(t *testing.T) {
		url := "/internal/v1/pets/" + uuid.New().String() + "/link?userA=" + uuid.New().String() + "&userB=" + uuid.New().String()
		req := httptest.NewRequest(http.MethodGet, url, nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test ล้มเหลว: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("ต้องการ 401 ได้ %d", resp.StatusCode)
		}
	})

	t.Run("/internal/v1/pets/:id/link ยอมรับ service token โดยไม่ต้องมี JWT เลย", func(t *testing.T) {
		url := "/internal/v1/pets/" + uuid.New().String() + "/link?userA=" + uuid.New().String() + "&userB=" + uuid.New().String()
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set(middleware.HeaderServiceToken, serviceToken)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test ล้มเหลว: %v", err)
		}
		// ⚠️ จุดสำคัญของเทสนี้ — ต้องไม่ใช่ 401 จาก JWT middleware
		// (ไม่สน DB error เพราะ db เป็น nil ในเทสนี้ — สนแค่ว่าผ่านด่าน auth ถูกต้อง)
		if resp.StatusCode == http.StatusUnauthorized {
			t.Fatal("internal route ต้องไม่โดน JWT middleware ครอบ ได้ 401 ทั้งที่ส่ง service token แล้ว")
		}
	})
}
