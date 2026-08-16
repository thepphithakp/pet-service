package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/vertex/pet-service/internal/domain"
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
	caregivers, err := h.useCase.GetForPet(c.Context(), petID)
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

	var body struct {
		UserID string `json:"userId"`
	}
	if err := c.BodyParser(&body); err != nil {
		return apperror.BadRequest("Invalid request body", err)
	}

	userID, err := uuid.Parse(body.UserID)
	if err != nil {
		return apperror.BadRequest("Invalid user ID", err)
	}

	caregiver, err := h.useCase.Add(c.Context(), petID, userID)
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

	var body struct {
		Permissions []domain.PetPermission `json:"permissions"`
	}
	if err := c.BodyParser(&body); err != nil {
		return apperror.BadRequest("Invalid request body", err)
	}

	updated, err := h.useCase.UpdatePermissions(c.Context(), caregiverID, body.Permissions)
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
	if err := h.useCase.Remove(c.Context(), caregiverID); err != nil {
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
