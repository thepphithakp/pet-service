//go:build integration

package bootstrap

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/config"
	"github.com/vertex/pet-service/pkg/middleware"
)

// -update เขียนไฟล์ golden ใหม่ ใช้ตอนที่ตั้งใจเปลี่ยนรูปแบบ error จริงๆ
//
//	go test -tags=integration -run TestErrorResponseShape ./internal/bootstrap/ -update
var updateGolden = flag.Bool("update", false, "เขียนไฟล์ golden ใหม่")

// TestErrorResponseShape ตรึงรูปแบบของ error response ไว้
//
// แอป iOS อ่าน field `error` กับ `requestId` (ดู APIErrorResponse ใน
// NetworkManager.swift) ถ้ารูปแบบเปลี่ยนโดยไม่ตั้งใจ — เปลี่ยนชื่อ field,
// เพิ่ม field ที่มีข้อมูลภายใน, หรือทำให้ requestId หายไป — แอปจะแสดง
// ข้อความผิดหรือ decode ไม่ผ่าน โดยไม่มีเทสต์ไหนจับได้
//
// เทสต์นี้เทียบ "โครง" ไม่ใช่ค่า เพราะ requestId เปลี่ยนทุก request
func TestErrorResponseShape(t *testing.T) {
	db := openTestDB(t)
	owner := uuid.New()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("สร้างคีย์ไม่สำเร็จ: %v", err)
	}
	app, _, _, _ := NewApp(db, config.Config{Port: "0"},
		middleware.AuthConfig{PublicKeys: []*rsa.PublicKey{&key.PublicKey}})

	petID := seedPet(t, db, owner)
	t.Cleanup(func() {
		db.Exec("DELETE FROM event_outbox WHERE entity_id = ?", petID.String())
		db.Exec("DELETE FROM water_logs WHERE pet_id = ?", petID)
		db.Exec("DELETE FROM pets WHERE id = ?", petID)
	})

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		noAuth bool
		as     *uuid.UUID
	}{
		{name: "401_ไม่มี_token", method: "GET", path: "/api/v1/pets", noAuth: true},
		{name: "404_ไม่พบสัตว์เลี้ยง", method: "GET", path: "/api/v1/pets/" + uuid.NewString()},
		{name: "400_id_ไม่ใช่_uuid", method: "GET", path: "/api/v1/pets/ไม่ใช่-uuid"},
		{
			name: "400_body_ไม่ถูกต้อง", method: "POST",
			path: fmt.Sprintf("/api/v1/pets/%s/water-logs", petID), body: `{`,
		},
		{
			name: "400_validation_ไม่ผ่าน", method: "POST",
			path: fmt.Sprintf("/api/v1/pets/%s/water-logs", petID), body: `{"amount":0}`,
		},
		{
			name: "400_master_data_ไม่รู้จัก", method: "POST",
			path: fmt.Sprintf("/api/v1/pets/%s/litter-logs", petID),
			body: `{"type":"ไม่มีชนิดนี้","amount":1}`,
		},
		{
			name: "400_cursor_ผิดรูปแบบ", method: "GET",
			path: fmt.Sprintf("/api/v1/pets/%s/water-logs?cursor=!!!", petID),
		},
		{name: "403_ไม่มีสิทธิ์", method: "GET", path: "/api/v1/admin/pets"},
		{name: "404_route_ไม่มีอยู่", method: "GET", path: "/api/v1/ไม่มี-route-นี้"},
	}

	got := map[string]any{}

	for _, tc := range cases {
		var status int
		var body []byte
		if tc.noAuth {
			status, body = doUnauthenticated(t, app, tc.method, tc.path)
		} else {
			status, body = doJSONAs(t, app, tc.method, tc.path, tc.body, key, owner)
		}

		if status < 400 {
			t.Fatalf("%s: status = %d ต้องเป็น error", tc.name, status)
		}

		var parsed map[string]any
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatalf("%s: response ต้องเป็น JSON object เสมอ ได้ %s", tc.name, body)
		}

		got[tc.name] = map[string]any{
			"status": status,
			"fields": shapeOf(parsed),
		}
	}

	compareGolden(t, "error_shape.json", got)
}

// shapeOf คืนรายชื่อ field พร้อมชนิด เรียงแล้ว
//
// เก็บแค่โครงไม่เก็บค่า เพราะ requestId เปลี่ยนทุกครั้ง
// และข้อความ error อาจปรับถ้อยคำได้โดยไม่กระทบสัญญากับ client
func shapeOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		kind := "null"
		switch v.(type) {
		case string:
			kind = "string"
		case float64:
			kind = "number"
		case bool:
			kind = "bool"
		case map[string]any:
			kind = "object"
		case []any:
			kind = "array"
		}
		out = append(out, k+":"+kind)
	}
	sort.Strings(out)
	return out
}

func compareGolden(t *testing.T, name string, got any) {
	t.Helper()

	path := filepath.Join("testdata", name)
	fresh, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("แปลงเป็น JSON ไม่ได้: %v", err)
	}
	fresh = append(fresh, '\n')

	if *updateGolden {
		if err := os.WriteFile(path, fresh, 0o644); err != nil {
			t.Fatalf("เขียนไฟล์ golden ไม่ได้: %v", err)
		}
		t.Logf("เขียน %s ใหม่แล้ว", path)
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("อ่านไฟล์ golden ไม่ได้ (รันด้วย -update เพื่อสร้างครั้งแรก): %v", err)
	}

	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(fresh)) {
		t.Errorf(`รูปแบบ error response เปลี่ยนไปจากที่ตรึงไว้

แอป iOS อ่าน field "error" กับ "requestId" — ถ้าตั้งใจเปลี่ยนจริง
ให้รันคำสั่งนี้แล้วตรวจ diff ก่อน commit
  go test -tags=integration -run TestErrorResponseShape ./internal/bootstrap/ -update

--- ที่ตรึงไว้ ---
%s
--- ที่ได้จริง ---
%s`, want, fresh)
	}
}

// TestErrorResponse_AlwaysHasContractFields
//
// ตรวจสัญญาขั้นต่ำแยกจาก golden — golden จับ "การเปลี่ยนแปลง"
// ส่วนอันนี้จับ "การผิดสัญญา" ซึ่งต้องไม่มีทางผ่านแม้จะอัปเดต golden แล้ว
func TestErrorResponse_AlwaysHasContractFields(t *testing.T) {
	db := openTestDB(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("สร้างคีย์ไม่สำเร็จ: %v", err)
	}
	app, _, _, _ := NewApp(db, config.Config{Port: "0"},
		middleware.AuthConfig{PublicKeys: []*rsa.PublicKey{&key.PublicKey}})

	status, body := doJSONAs(t, app, "GET", "/api/v1/pets/"+uuid.NewString(), "", key, uuid.New())
	if status != fiber.StatusNotFound {
		t.Fatalf("status = %d", status)
	}

	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("ต้องเป็น JSON: %v", err)
	}

	msg, ok := parsed["error"].(string)
	if !ok || msg == "" {
		t.Error(`ต้องมี field "error" เป็น string ที่ไม่ว่าง — แอปเอาไปแสดงให้ผู้ใช้`)
	}
	if _, ok := parsed["requestId"].(string); !ok {
		t.Error(`ต้องมี field "requestId" เป็น string — ใช้ไล่ปัญหาข้าม service`)
	}

	// ห้ามมีข้อมูลภายในหลุดไปกับ error
	for _, forbidden := range []string{"cause", "stack", "sql", "query", "detail"} {
		if _, exists := parsed[forbidden]; exists {
			t.Errorf("field %q ไม่ควรอยู่ใน response — เป็นข้อมูลภายใน", forbidden)
		}
	}

	lower := strings.ToLower(string(body))
	for _, leak := range []string{"pgx", "gorm", "sqlstate", "select ", "postgres"} {
		if strings.Contains(lower, leak) {
			t.Errorf("รายละเอียดภายในหลุดใน error: พบ %q", leak)
		}
	}
}
