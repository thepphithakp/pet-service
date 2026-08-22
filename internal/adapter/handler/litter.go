package handler

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/adapter/handler/dto"
	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
	"github.com/vertex/pet-service/pkg/apperror"
)

// maxBatchSize จำกัดขนาด batch เพื่อไม่ให้ request เดียวกินหน่วยความจำจนล้ม
const maxBatchSize = 500

// LitterHandler handles HTTP requests for litter log operations.
type LitterHandler struct {
	useCase port.LitterUseCase
}

func NewLitterHandler(uc port.LitterUseCase) *LitterHandler {
	return &LitterHandler{useCase: uc}
}

func (h *LitterHandler) GetAll(c *fiber.Ctx) error {
	petID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.BadRequest("Invalid pet ID", err)
	}
	logs, err := h.useCase.GetForPet(c.UserContext(), petID)
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.JSON(logs)
}

func (h *LitterHandler) Create(c *fiber.Ctx) error {
	petID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.BadRequest("Invalid pet ID", err)
	}

	var req dto.LitterLogRequest
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

func (h *LitterHandler) CreateBatch(c *fiber.Ctx) error {
	petID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.BadRequest("Invalid pet ID", err)
	}

	var reqs []dto.BatchLitterLogRequest
	if err := c.BodyParser(&reqs); err != nil {
		return apperror.BadRequest("Invalid request body (expected array)", err)
	}
	if len(reqs) > maxBatchSize {
		return apperror.BadRequest(fmt.Sprintf("ส่งได้สูงสุด %d รายการต่อครั้ง", maxBatchSize))
	}

	logs := make([]domain.LitterLog, len(reqs))
	for i, r := range reqs {
		if err := r.Validate(); err != nil {
			return apperror.FromDomain(fmt.Errorf("รายการที่ %d — %w", i+1, err))
		}
		logs[i] = r.ToDomain()
		logs[i].PetID = petID
		setLogActor(c, &logs[i].CreatedBy, &logs[i].CreatedByUsername)
	}

	created, err := h.useCase.CreateBatch(c.UserContext(), logs)
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.Status(fiber.StatusCreated).JSON(created)
}

func (h *LitterHandler) Delete(c *fiber.Ctx) error {
	petID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.BadRequest("Invalid pet ID", err)
	}
	logID, err := uuid.Parse(c.Params("logId"))
	if err != nil {
		return apperror.BadRequest("Invalid litter log ID", err)
	}
	if err := h.useCase.Delete(c.UserContext(), petID, logID); err != nil {
		return apperror.FromDomain(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *LitterHandler) RegisterRoutes(r fiber.Router) {
	r.Get("/pets/:id/litter-logs", h.GetAll)
	r.Post("/pets/:id/litter-logs", h.Create)
	r.Post("/pets/:id/litter-logs/batch", h.CreateBatch)
	r.Delete("/pets/:id/litter-logs/:logId", h.Delete)
}
