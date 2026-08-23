package handler

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/adapter/handler/dto"
	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
	"github.com/vertex/pet-service/pkg/apperror"
)

// PetHandler handles HTTP requests for pet operations.
type PetHandler struct {
	useCase port.PetUseCase

	// listIncludesAvatar เป็นสวิตช์ชั่วคราวสำหรับช่วงเปลี่ยนผ่าน
	//
	// GET /pets เดิมส่ง avatarData ไปกับทุกรายการ ซึ่งบน production
	// แปลว่าผู้ใช้ที่มีสัตว์เลี้ยง 3 ตัวต้องโหลดเกือบ 4MB ทุกครั้งที่เปิดหน้ารายการ
	//
	// แอปที่ใช้อยู่ตอนนี้อ่านรูปจาก field นี้ ถ้าตัดออกทันทีรูปจะหายจากแอป
	// จึงเปิดค่านี้ไว้ก่อน (พฤติกรรมเดิม) แล้วปิดเมื่อแอปเปลี่ยนไปใช้
	// GET /pets/:id/avatar แล้ว — ปิดได้ด้วย PET_LIST_INCLUDE_AVATAR=false
	// โดยไม่ต้อง build ใหม่
	listIncludesAvatar bool
}

func NewPetHandler(uc port.PetUseCase, listIncludesAvatar bool) *PetHandler {
	return &PetHandler{useCase: uc, listIncludesAvatar: listIncludesAvatar}
}

func (h *PetHandler) GetAll(c *fiber.Ctx) error {
	actor, ok := domain.ActorFromContext(c.UserContext())
	if !ok {
		return apperror.Unauthorized("Missing user ID in token")
	}

	if h.listIncludesAvatar {
		pets, err := h.useCase.GetAllForUser(c.UserContext(), actor.UserID)
		if err != nil {
			return apperror.FromDomain(err)
		}
		return c.JSON(pets)
	}

	pets, err := h.useCase.GetAllForUserSummary(c.UserContext(), actor.UserID)
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.JSON(pets)
}

func (h *PetHandler) AdminGetAll(c *fiber.Ctx) error {
	if h.listIncludesAvatar {
		pets, err := h.useCase.GetAll(c.UserContext())
		if err != nil {
			return apperror.FromDomain(err)
		}
		return c.JSON(pets)
	}

	pets, err := h.useCase.GetAllSummary(c.UserContext())
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.JSON(pets)
}

// GetAvatar คืนรูปเป็น binary พร้อมข้อมูลสำหรับ cache
//
// รองรับ conditional request: ผู้เรียกส่ง ETag ที่ได้จากครั้งก่อนมาใน
// If-None-Match ถ้ารูปไม่เปลี่ยนจะได้ 304 ที่ไม่มี body เลย
func (h *PetHandler) GetAvatar(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.BadRequest("Invalid pet ID", err)
	}

	avatar, err := h.useCase.GetAvatar(c.UserContext(), id)
	if err != nil {
		return apperror.FromDomain(err)
	}
	if avatar == nil {
		return apperror.NotFound("Avatar", nil)
	}

	c.Set(fiber.HeaderETag, avatar.ETag)
	// private เพราะรูปเป็นของผู้ใช้แต่ละคน ห้าม proxy กลางเก็บแล้วแจกต่อ
	// must-revalidate บังคับให้ถาม server ก่อนใช้ของเก่าเมื่อหมดอายุ
	c.Set(fiber.HeaderCacheControl, "private, max-age=300, must-revalidate")

	if match := c.Get(fiber.HeaderIfNoneMatch); match != "" && etagMatches(match, avatar.ETag) {
		return c.SendStatus(fiber.StatusNotModified)
	}

	// ไม่ได้เก็บ mime type ไว้ในฐานข้อมูล จึงเดาจากตัวข้อมูล
	c.Set(fiber.HeaderContentType, http.DetectContentType(avatar.Data))
	return c.Send(avatar.Data)
}

// etagMatches รองรับทั้งค่าเดียวและหลายค่าคั่นด้วยจุลภาค ตาม RFC 9110
// รวมถึง "*" ที่แปลว่า "ตรงกับอะไรก็ได้ที่มีอยู่"
func etagMatches(header, etag string) bool {
	header = strings.TrimSpace(header)
	if header == "*" {
		return true
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		// W/ นำหน้าคือ weak validator — เทียบเนื้อในเหมือนกัน
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == etag {
			return true
		}
	}
	return false
}

func (h *PetHandler) GetOne(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.BadRequest("Invalid pet ID", err)
	}
	pet, err := h.useCase.GetByID(c.UserContext(), id)
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.JSON(pet)
}

func (h *PetHandler) Create(c *fiber.Ctx) error {
	// C-1: เดิมเป็น type assertion แบบไม่มี ok → panic ได้ถ้า middleware ไม่ได้เซ็ต
	actor, ok := domain.ActorFromContext(c.UserContext())
	if !ok {
		return apperror.Unauthorized("Missing user ID in token")
	}

	var req dto.CreatePetRequest
	if err := c.BodyParser(&req); err != nil {
		return apperror.BadRequest("Invalid request body", err)
	}
	if err := req.Validate(); err != nil {
		return apperror.FromDomain(err)
	}

	pet := req.ToDomain()
	pet.OwnerUsername = actor.Username

	created, err := h.useCase.Create(c.UserContext(), &pet, actor.UserID)
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.Status(fiber.StatusCreated).JSON(created)
}

func (h *PetHandler) Update(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.BadRequest("Invalid pet ID", err)
	}

	var req dto.UpdatePetRequest
	if err := c.BodyParser(&req); err != nil {
		return apperror.BadRequest("Invalid request body", err)
	}
	if err := req.Validate(); err != nil {
		return apperror.FromDomain(err)
	}

	pet := req.ToDomain()
	updated, err := h.useCase.Update(c.UserContext(), id, &pet)
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.JSON(updated)
}

func (h *PetHandler) Delete(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.BadRequest("Invalid pet ID", err)
	}
	if err := h.useCase.Delete(c.UserContext(), id); err != nil {
		return apperror.FromDomain(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// RegisterRoutes ลงทะเบียน route ของผู้ใช้ทั่วไป
func (h *PetHandler) RegisterRoutes(router fiber.Router) {
	router.Get("/pets", h.GetAll)
	router.Get("/pets/:id", h.GetOne)
	router.Get("/pets/:id/avatar", h.GetAvatar)
	router.Post("/pets", h.Create)
	router.Put("/pets/:id", h.Update)
	router.Delete("/pets/:id", h.Delete)
}

// RegisterAdminRoutes ลงทะเบียนแยกจาก route ปกติ
//
// การตรวจสิทธิ์จริงอยู่ที่ชั้น service (PetService.GetAll) ไม่ใช่ที่ route group
// เพื่อไม่ให้ข้ามได้ถ้ามี caller อื่นเรียก use case ตรงๆ
func (h *PetHandler) RegisterAdminRoutes(router fiber.Router) {
	router.Get("/pets", h.AdminGetAll)
}
