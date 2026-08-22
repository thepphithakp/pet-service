package handler

import (
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
}

func NewPetHandler(uc port.PetUseCase) *PetHandler {
	return &PetHandler{useCase: uc}
}

func (h *PetHandler) GetAll(c *fiber.Ctx) error {
	actor, ok := domain.ActorFromContext(c.UserContext())
	if !ok {
		return apperror.Unauthorized("Missing user ID in token")
	}

	pets, err := h.useCase.GetAllForUser(c.UserContext(), actor.UserID)
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.JSON(pets)
}

func (h *PetHandler) AdminGetAll(c *fiber.Ctx) error {
	pets, err := h.useCase.GetAll(c.UserContext())
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.JSON(pets)
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
