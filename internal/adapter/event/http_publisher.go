package event

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/vertex/pet-service/internal/port"
)

// publishTimeout จำกัดเวลาที่ยอมรอ event-service
//
// เดิมใช้ http.Post กับ default client ซึ่ง "ไม่มี timeout เลย"
// ถ้า event-service ค้าง goroutine จะค้างตามไปเรื่อยๆ จนกินหน่วยความจำ
const publishTimeout = 5 * time.Second

// maxInFlight จำกัดจำนวน goroutine ที่ยิง event พร้อมกัน
//
// เดิมสร้าง goroutine ใหม่ทุกครั้งไม่มีเพดาน — ถ้า event-service ช้า
// จำนวน goroutine จะโตตามจำนวน request จนล้ม
const maxInFlight = 32

type HTTPEventPublisher struct {
	EventServiceURL string

	client *http.Client
	// slots ทำหน้าที่เป็นเพดานจำนวน goroutine ที่ทำงานพร้อมกัน
	slots chan struct{}
}

// NewHTTPEventPublisher รับ URL จากภายนอกแทนการอ่าน env เอง (แก้ A-5)
func NewHTTPEventPublisher(eventServiceURL string) *HTTPEventPublisher {
	return &HTTPEventPublisher{
		EventServiceURL: eventServiceURL,
		client:          &http.Client{Timeout: publishTimeout},
		slots:           make(chan struct{}, maxInFlight),
	}
}

// Publish ส่ง event แบบไม่บล็อก request หลัก
//
// 🔸 ยังเป็น fire-and-forget อยู่ — event หายได้ถ้า pod ถูกฆ่ากลางทาง
//
//	Phase 7.2 จะเปลี่ยนเป็น transactional outbox ที่เขียน event ลง
//	ฐานข้อมูลในทรานแซกชันเดียวกับข้อมูลธุรกิจ แล้วมี worker ส่งทีหลัง
func (p *HTTPEventPublisher) Publish(ctx context.Context, event port.EventLog) {
	// ตัดสายจาก ctx ของ request แต่เก็บค่าที่ผูกไว้ (เช่น request id)
	// ถ้าใช้ ctx เดิม request จบก่อน goroutine จะโดน cancel ทันที
	bg := context.WithoutCancel(ctx)

	select {
	case p.slots <- struct{}{}:
	default:
		// เพดานเต็ม — ทิ้ง event นี้ดีกว่าปล่อยให้ goroutine โตไม่จำกัด
		slog.WarnContext(ctx, "ทิ้ง event เพราะคิวเต็ม",
			"eventType", event.EventType, "entityId", event.EntityID)
		return
	}

	go func() {
		defer func() { <-p.slots }()

		reqCtx, cancel := context.WithTimeout(bg, publishTimeout)
		defer cancel()

		if err := p.send(reqCtx, event); err != nil {
			slog.ErrorContext(reqCtx, "ส่ง event ไม่สำเร็จ",
				"eventType", event.EventType, "entityId", event.EntityID, "error", err)
		}
	}()
}

func (p *HTTPEventPublisher) send(ctx context.Context, event port.EventLog) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.EventServiceURL+"/api/v1/events", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return &unexpectedStatusError{status: resp.StatusCode}
	}
	return nil
}

type unexpectedStatusError struct{ status int }

func (e *unexpectedStatusError) Error() string {
	return "event-service ตอบ status " + http.StatusText(e.status)
}
