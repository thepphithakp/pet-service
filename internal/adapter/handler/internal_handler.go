package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/port"
	"github.com/vertex/pet-service/pkg/apperror"
)

// InternalHandler รวม endpoint ที่ service อื่นเรียก (ไม่ใช่ผู้ใช้โดยตรง)
//
// แยกออกจาก handler ที่เหลือทั้งหมดโดยตั้งใจ — ผูกกับ middleware.NewServiceToken
// ไม่ใช่ JWT และ bootstrap/routes.go ต้องลงทะเบียน route กลุ่มนี้แยกจาก
// app.Group("/api/v1", authMW) เดิม (ห้ามใช้ path ที่ขึ้นต้นด้วย "/api/v1" เลย
// เพราะ group นั้นครอบทุก path ที่ขึ้นต้นด้วย "/api/v1" ด้วย string-prefix
// match ของ Fiber — ไม่ใช่แค่ sub-path ที่ตั้งใจ)
//
// 🔴 handler นี้คุยกับ port.PetLinkUseCase (application layer) เท่านั้น
// ไม่คุยกับ repository ตรงๆ — เดิมเคยเรียก FindAccess สองครั้งแล้วเทียบเอง
// ในชั้นนี้ ทำให้ business rule "เกี่ยวข้องกับสัตว์เลี้ยงทั้งคู่ไหม" ไม่มีที่
// อยู่ที่ application เลย ย้ายไปที่ application.PetLinkService แล้ว
type InternalHandler struct {
	petLinks port.PetLinkUseCase
}

func NewInternalHandler(petLinks port.PetLinkUseCase) *InternalHandler {
	return &InternalHandler{petLinks: petLinks}
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
		return apperror.BadRequest("petId ต้องเป็น UUID")
	}
	userA, err := uuid.Parse(c.Query("userA"))
	if err != nil {
		return apperror.BadRequest("userA ต้องเป็น UUID")
	}
	userB, err := uuid.Parse(c.Query("userB"))
	if err != nil {
		return apperror.BadRequest("userB ต้องเป็น UUID")
	}

	linked, err := h.petLinks.IsLinked(c.UserContext(), petID, userA, userB)
	if err != nil {
		return apperror.Internal("ตรวจสอบความเกี่ยวข้องกับสัตว์เลี้ยงไม่สำเร็จ", err)
	}
	return c.JSON(fiber.Map{"linked": linked})
}
