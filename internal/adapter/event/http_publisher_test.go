package event

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/vertex/pet-service/internal/port"
)

type captured struct {
	token string
	body  map[string]any
}

// recorder รับ request ที่ publisher ยิงมา แล้วเก็บไว้ให้เทสต์ตรวจ
func recorder(t *testing.T) (*httptest.Server, func() []captured) {
	t.Helper()
	var (
		mu   sync.Mutex
		got  []captured
		done = make(chan struct{}, 16)
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)

		mu.Lock()
		got = append(got, captured{token: r.Header.Get(serviceTokenHeader), body: body})
		mu.Unlock()

		w.WriteHeader(http.StatusCreated)
		done <- struct{}{}
	}))
	t.Cleanup(srv.Close)

	wait := func() []captured {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("รอ request ไม่มาภายในเวลาที่กำหนด")
		}
		mu.Lock()
		defer mu.Unlock()
		out := make([]captured, len(got))
		copy(out, got)
		return out
	}
	return srv, wait
}

// TestPublish_SendsServiceToken
//
// event-service ปฏิเสธ request ที่ไม่มี header นี้ ถ้า publisher ไม่ส่ง
// event ทุกตัวจะหายเงียบๆ โดยผู้ใช้ไม่เห็นอะไรผิดปกติ
func TestPublish_SendsServiceToken(t *testing.T) {
	srv, wait := recorder(t)
	p := NewHTTPEventPublisher(srv.URL, "test-ingest-token")

	p.Publish(context.Background(), port.EventLog{
		EventType: "WaterLog", Action: "Water Intake Logged", EntityID: "pet-1",
	})

	got := wait()
	if got[0].token != "test-ingest-token" {
		t.Errorf("X-Service-Token = %q ต้องเป็น test-ingest-token", got[0].token)
	}
}

// TestPublish_SendsIdempotencyKey
func TestPublish_SendsIdempotencyKey(t *testing.T) {
	srv, wait := recorder(t)
	p := NewHTTPEventPublisher(srv.URL, "tok")

	p.Publish(context.Background(), port.EventLog{EventType: "x", Action: "y"})

	got := wait()
	key, ok := got[0].body["idempotencyKey"].(string)
	if !ok || key == "" {
		t.Fatalf("ต้องส่ง idempotencyKey มาด้วย: %v", got[0].body)
	}
	if got[0].body["eventType"] != "x" {
		t.Errorf("field เดิมต้องยังอยู่ครบ: %v", got[0].body)
	}
}

// TestPublish_EachEventGetsDistinctKey
//
// สอง event คนละตัวต้องไม่ใช้คีย์เดียวกัน ไม่งั้นตัวที่สองจะถูกมองว่าซ้ำ
// แล้วหายไปจาก log
func TestPublish_EachEventGetsDistinctKey(t *testing.T) {
	srv, wait := recorder(t)
	p := NewHTTPEventPublisher(srv.URL, "tok")

	p.Publish(context.Background(), port.EventLog{EventType: "a", Action: "a"})
	wait()
	p.Publish(context.Background(), port.EventLog{EventType: "b", Action: "b"})
	got := wait()

	if len(got) != 2 {
		t.Fatalf("ได้ %d request ต้องได้ 2", len(got))
	}
	if got[0].body["idempotencyKey"] == got[1].body["idempotencyKey"] {
		t.Error("event คนละตัวต้องได้คีย์คนละค่า")
	}
}

// TestPublish_NoTokenConfigured ไม่ตั้ง token ต้องไม่ทำให้ pod พัง
func TestPublish_NoTokenConfigured(t *testing.T) {
	srv, wait := recorder(t)
	p := NewHTTPEventPublisher(srv.URL, "")

	p.Publish(context.Background(), port.EventLog{EventType: "x", Action: "y"})

	got := wait()
	if got[0].token != "" {
		t.Errorf("ไม่ได้ตั้ง token ต้องไม่ส่ง header มา: %q", got[0].token)
	}
}

// TestSend_ReturnsErrorUnlikePublish
//
// outbox worker ใช้ Send เพราะต้องรู้ว่าสำเร็จไหมจึงจะ mark ว่าส่งแล้วได้
// ต่างจาก Publish ที่กลืน error ทิ้ง
func TestSend_ReturnsErrorUnlikePublish(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{"201 = สำเร็จ", http.StatusCreated, false},
		{"200 = สำเร็จ (event ซ้ำ)", http.StatusOK, false},
		{"401 = ล้มเหลว", http.StatusUnauthorized, true},
		{"500 = ล้มเหลว", http.StatusInternalServerError, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			p := NewHTTPEventPublisher(srv.URL, "tok")
			err := p.Send(context.Background(), port.EventLog{EventType: "x", Action: "y"}, "key-1")

			if tc.wantErr && err == nil {
				t.Error("ต้องคืน error เพื่อให้ worker รู้ว่าต้อง retry")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ไม่ควร error: %v", err)
			}
		})
	}
}

// TestSend_UsesGivenIdempotencyKey
//
// worker ส่งคีย์เดิมทุกครั้งที่ retry ถ้า Send ไปสร้างใหม่เองจะกลายเป็น
// event ซ้ำที่ปลายทาง
func TestSend_UsesGivenIdempotencyKey(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		got, _ = body["idempotencyKey"].(string)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	p := NewHTTPEventPublisher(srv.URL, "tok")
	if err := p.Send(context.Background(), port.EventLog{EventType: "x", Action: "y"}, "คีย์เดิม"); err != nil {
		t.Fatalf("ไม่ควร error: %v", err)
	}
	if got != "คีย์เดิม" {
		t.Errorf("idempotencyKey = %q ต้องเป็นค่าที่ส่งเข้ามา", got)
	}
}

// TestUnexpectedStatusError_Message
func TestUnexpectedStatusError_Message(t *testing.T) {
	err := &unexpectedStatusError{status: http.StatusBadGateway}
	if err.Error() == "" {
		t.Error("ข้อความ error ต้องไม่ว่าง")
	}
}
