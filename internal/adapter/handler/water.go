package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/adapter/handler/dto"
	"github.com/vertex/pet-service/internal/port"
	"github.com/vertex/pet-service/pkg/apperror"
)

type WaterHandler struct {
	useCase port.WaterUseCase
}

func NewWaterHandler(uc port.WaterUseCase) *WaterHandler {
	return &WaterHandler{useCase: uc}
}

func (h *WaterHandler) GetAll(c *fiber.Ctx) error {
	petID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.BadRequest("Invalid pet ID", err)
	}
	logs, err := h.useCase.GetByPetID(c.UserContext(), petID)
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.JSON(logs)
}

func (h *WaterHandler) Create(c *fiber.Ctx) error {
	petID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.BadRequest("Invalid pet ID", err)
	}

	var req dto.WaterLogRequest
	if err := c.BodyParser(&req); err != nil {
		return apperror.BadRequest("Invalid request body", err)
	}
	if err := req.Validate(); err != nil {
		return apperror.FromDomain(err)
	}

	log := req.ToDomain()
	log.PetID = petID
	setLogActor(c, &log.CreatedBy, &log.CreatedByUsername)

	created, err := h.useCase.Create(c.UserContext(), &log)
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.Status(fiber.StatusCreated).JSON(created)
}

func (h *WaterHandler) Delete(c *fiber.Ctx) error {
	petID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.BadRequest("Invalid pet ID", err)
	}
	logID, err := uuid.Parse(c.Params("logId"))
	if err != nil {
		return apperror.BadRequest("Invalid water log ID", err)
	}
	if err := h.useCase.Delete(c.UserContext(), petID, logID); err != nil {
		return apperror.FromDomain(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *WaterHandler) RegisterRoutes(r fiber.Router) {
	r.Get("/pets/:id/water-logs", h.GetAll)
	r.Post("/pets/:id/water-logs", h.Create)
	r.Delete("/pets/:id/water-logs/:logId", h.Delete)
}
