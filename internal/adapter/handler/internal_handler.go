package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/pkg/middleware"
)

// PetAccessChecker คือส่วนเล็กๆ ของ port.PetRepository ที่ endpoint นี้ต้องใช้
// จริง — ประกาศแยกแทนที่จะพึ่ง interface เต็มของ PetRepository เพื่อให้ fake
// ตอนเทสไม่ต้อง implement เมธอดที่ไม่เกี่ยวข้องอีกสิบกว่าตัว
// (*repository.GORMPetRepository ที่ใช้งานจริงมี FindAccess อยู่แล้ว จึง
// implement interface นี้โดยอัตโนมัติไม่ต้องแก้อะไรที่ wire())
type PetAccessChecker interface {
	FindAccess(ctx context.Context, petID, userID uuid.UUID) (domain.PetAccess, error)
}

// InternalHandler รวม endpoint ที่ service อื่นเรียก (ไม่ใช่ผู้ใช้โดยตรง)
//
// แยกออกจาก handler ที่เหลือทั้งหมดโดยตั้งใจ — ผูกกับ middleware.NewServiceToken
// ไม่ใช่ JWT และ bootstrap/routes.go ต้องลงทะเบียน route กลุ่มนี้แยกจาก
// app.Group("/api/v1", authMW) เดิม (ห้ามใช้ path ที่ขึ้นต้นด้วย "/api/v1" เลย
// เพราะ group นั้นครอบทุก path ที่ขึ้นต้นด้วย "/api/v1" ด้วย string-prefix
// match ของ Fiber — ไม่ใช่แค่ sub-path ที่ตั้งใจ)
type InternalHandler struct {
	pets PetAccessChecker
}

func NewInternalHandler(pets PetAccessChecker) *InternalHandler {
	return &InternalHandler{pets: pets}
}

func (h *InternalHandler) RegisterRoutes(r fiber.Router) {
	r.Get("/pets/:petId/link", h.PetLink)
}

// PetLink ตอบว่าผู้ใช้สองคนนี้ "เกี่ยวข้องกับสัตว์เลี้ยงตัวนี้ทั้งคู่" ไหม
// (เจ้าของหรือผู้ดูแลก็นับ) — chat-service เรียกมาถามก่อนเปิดบทสนทนาใหม่
// เพื่อจำกัดไม่ให้ user สองคนที่ไม่เกี่ยวข้องกันเลยคุยกันได้
func (h *InternalHandler) PetLink(c *fiber.Ctx) error {
	petID, err := uuid.Parse(c.Params("petId"))
	if err != nil {
		return badRequestInternal(c, "petId ต้องเป็น UUID")
	}
	userA, err := uuid.Parse(c.Query("userA"))
	if err != nil {
		return badRequestInternal(c, "userA ต้องเป็น UUID")
	}
	userB, err := uuid.Parse(c.Query("userB"))
	if err != nil {
		return badRequestInternal(c, "userB ต้องเป็น UUID")
	}

	accessA, err := h.pets.FindAccess(c.UserContext(), petID, userA)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	accessB, err := h.pets.FindAccess(c.UserContext(), petID, userB)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	linked := accessA.Level != domain.AccessNone && accessB.Level != domain.AccessNone
	return c.JSON(fiber.Map{"linked": linked})
}

func badRequestInternal(c *fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
		"error": msg, "requestId": c.Get(middleware.HeaderRequestID),
	})
}
