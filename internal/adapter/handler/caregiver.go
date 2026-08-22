package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/adapter/handler/dto"
	"github.com/vertex/pet-service/internal/port"
	"github.com/vertex/pet-service/pkg/apperror"
)

// CaregiverHandler handles HTTP requests for caregiver operations.
type CaregiverHandler struct {
	useCase port.CaregiverUseCase
}

func NewCaregiverHandler(uc port.CaregiverUseCase) *CaregiverHandler {
	return &CaregiverHandler{useCase: uc}
}

func (h *CaregiverHandler) GetAll(c *fiber.Ctx) error {
	petID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.BadRequest("Invalid pet ID", err)
	}
	caregivers, err := h.useCase.GetForPet(c.UserContext(), petID)
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.JSON(caregivers)
}

func (h *CaregiverHandler) Add(c *fiber.Ctx) error {
	petID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.BadRequest("Invalid pet ID", err)
	}

	var body dto.AddCaregiverRequest
	if err := c.BodyParser(&body); err != nil {
		return apperror.BadRequest("Invalid request body", err)
	}

	userID, err := uuid.Parse(body.UserID)
	if err != nil {
		return apperror.BadRequest("Invalid user ID", err)
	}

	caregiver, err := h.useCase.Add(c.UserContext(), petID, userID)
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.Status(fiber.StatusCreated).JSON(caregiver)
}

func (h *CaregiverHandler) UpdatePermissions(c *fiber.Ctx) error {
	caregiverID, err := uuid.Parse(c.Params("caregiverId"))
	if err != nil {
		return apperror.BadRequest("Invalid caregiver ID", err)
	}

	// S-4: รับแค่ ID ไม่รับ object เต็มก้อน — ไม่งั้น client แก้ master data ได้
	var body dto.UpdatePermissionsRequest
	if err := c.BodyParser(&body); err != nil {
		return apperror.BadRequest("Invalid request body", err)
	}

	updated, err := h.useCase.UpdatePermissions(c.UserContext(), caregiverID, body.IDs())
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.JSON(updated)
}

func (h *CaregiverHandler) Remove(c *fiber.Ctx) error {
	caregiverID, err := uuid.Parse(c.Params("caregiverId"))
	if err != nil {
		return apperror.BadRequest("Invalid caregiver ID", err)
	}
	if err := h.useCase.Remove(c.UserContext(), caregiverID); err != nil {
		return apperror.FromDomain(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *CaregiverHandler) RegisterRoutes(r fiber.Router) {
	r.Get("/pets/:id/caregivers", h.GetAll)
	r.Post("/pets/:id/caregivers", h.Add)
	r.Put("/pets/:id/caregivers/:caregiverId", h.UpdatePermissions)
	r.Delete("/pets/:id/caregivers/:caregiverId", h.Remove)
}
