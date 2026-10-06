package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
)

type fakeAccessChecker struct {
	access map[uuid.UUID]domain.PetAccess // keyed by userID
}

func (f *fakeAccessChecker) FindAccess(_ context.Context, _, userID uuid.UUID) (domain.PetAccess, error) {
	return f.access[userID], nil
}

func newInternalTestApp(checker PetAccessChecker) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: func(c *fiber.Ctx, err error) error {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}})
	h := NewInternalHandler(checker)
	h.RegisterRoutes(app.Group("/internal/v1"))
	return app
}

func TestInternalPetLink_BothLinked(t *testing.T) {
	owner := uuid.New()
	caregiver := uuid.New()
	petID := uuid.New()

	app := newInternalTestApp(&fakeAccessChecker{access: map[uuid.UUID]domain.PetAccess{
		owner:     {Level: domain.AccessOwner},
		caregiver: {Level: domain.AccessCaregiver},
	}})

	req := httptest.NewRequest("GET", "/internal/v1/pets/"+petID.String()+"/link?userA="+owner.String()+"&userB="+caregiver.String(), nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !decodeLinked(t, resp) {
		t.Fatal("เจ้าของกับผู้ดูแลของสัตว์เลี้ยงตัวเดียวกัน ต้องได้ linked=true")
	}
}

func TestInternalPetLink_RejectsWhenOneSideUnrelated(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()
	petID := uuid.New()

	app := newInternalTestApp(&fakeAccessChecker{access: map[uuid.UUID]domain.PetAccess{
		owner: {Level: domain.AccessOwner},
		// stranger ไม่มีใน map เลย — FindAccess คืน zero value = AccessNone
	}})

	req := httptest.NewRequest("GET", "/internal/v1/pets/"+petID.String()+"/link?userA="+owner.String()+"&userB="+stranger.String(), nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d (endpoint เองต้องตอบ 200 เสมอ ผลอยู่ใน body.linked)", resp.StatusCode)
	}
	if decodeLinked(t, resp) {
		t.Fatal("stranger ไม่เกี่ยวข้องกับสัตว์เลี้ยงตัวนี้เลย ต้องได้ linked=false")
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
	app := newInternalTestApp(&fakeAccessChecker{access: map[uuid.UUID]domain.PetAccess{}})

	req := httptest.NewRequest("GET", "/internal/v1/pets/not-a-uuid/link?userA=x&userB=y", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d ต้องการ 400", resp.StatusCode)
	}
}
