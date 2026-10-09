package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/pkg/middleware"
)

type fakePetLinkUseCase struct {
	linked bool
	err    error
}

func (f *fakePetLinkUseCase) IsLinked(_ context.Context, _, _, _ uuid.UUID) (bool, error) {
	return f.linked, f.err
}

func newInternalTestApp(uc *fakePetLinkUseCase) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: middleware.ErrorHandler})
	h := NewInternalHandler(uc)
	h.RegisterRoutes(app.Group("/internal/v1"))
	return app
}

func TestInternalPetLink_BothLinked(t *testing.T) {
	petID, userA, userB := uuid.New(), uuid.New(), uuid.New()
	app := newInternalTestApp(&fakePetLinkUseCase{linked: true})

	req := httptest.NewRequest("GET", "/internal/v1/pets/"+petID.String()+"/link?userA="+userA.String()+"&userB="+userB.String(), nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !decodeLinked(t, resp) {
		t.Fatal("use case ตอบ linked=true ต้องได้ linked=true กลับมา")
	}
}

func TestInternalPetLink_NotLinked(t *testing.T) {
	petID, userA, userB := uuid.New(), uuid.New(), uuid.New()
	app := newInternalTestApp(&fakePetLinkUseCase{linked: false})

	req := httptest.NewRequest("GET", "/internal/v1/pets/"+petID.String()+"/link?userA="+userA.String()+"&userB="+userB.String(), nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d (endpoint เองต้องตอบ 200 เสมอ ผลอยู่ใน body.linked)", resp.StatusCode)
	}
	if decodeLinked(t, resp) {
		t.Fatal("use case ตอบ linked=false ต้องได้ linked=false กลับมา")
	}
}

func TestInternalPetLink_UseCaseErrorBecomes500ViaApperror(t *testing.T) {
	petID, userA, userB := uuid.New(), uuid.New(), uuid.New()
	app := newInternalTestApp(&fakePetLinkUseCase{err: errors.New("db ล่ม")})

	req := httptest.NewRequest("GET", "/internal/v1/pets/"+petID.String()+"/link?userA="+userA.String()+"&userB="+userB.String(), nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("status = %d ต้องการ 500", resp.StatusCode)
	}
	var body struct {
		Error     string `json:"error"`
		RequestID string `json:"requestId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error == "" {
		t.Fatal("ต้องตอบผ่าน apperror shape เดียวกับ endpoint อื่น (error + requestId)")
	}
}

func decodeLinked(t *testing.T, resp *http.Response) bool {
	t.Helper()
	var body struct {
		Linked bool `json:"linked"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Linked
}

func TestInternalPetLink_RejectsInvalidUUID(t *testing.T) {
	app := newInternalTestApp(&fakePetLinkUseCase{})

	req := httptest.NewRequest("GET", "/internal/v1/pets/not-a-uuid/link?userA=x&userB=y", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d ต้องการ 400", resp.StatusCode)
	}
}
