package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
	"github.com/vertex/pet-service/pkg/apperror"
)

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
	logs, err := h.useCase.GetForPet(c.Context(), petID)
	if err != nil {
		return apperror.Internal("Failed to fetch litter logs", err)
	}
	return c.JSON(logs)
}

func (h *LitterHandler) Create(c *fiber.Ctx) error {
	petID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.BadRequest("Invalid pet ID", err)
	}

	var log domain.LitterLog
	if err := c.BodyParser(&log); err != nil {
		return apperror.BadRequest("Invalid request body", err)
	}
	log.PetID = petID

	userId, _ := c.Locals("userId").(string)
	userName, _ := c.Locals("userName").(string)
	if userId != "" {
		log.CreatedBy = &userId
	}
	if userName != "" {
		log.CreatedByUsername = &userName
	}
	created, err := h.useCase.Create(c.Context(), &log)
	if err != nil {
		return apperror.Internal("Failed to create litter log", err)
	}
	return c.Status(fiber.StatusCreated).JSON(created)
}

func (h *LitterHandler) CreateBatch(c *fiber.Ctx) error {
	petID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.BadRequest("Invalid pet ID", err)
	}

	var logs []domain.LitterLog
	if err := c.BodyParser(&logs); err != nil {
		return apperror.BadRequest("Invalid request body (expected array)", err)
	}
	userId, _ := c.Locals("userId").(string)
	userName, _ := c.Locals("userName").(string)

	for i := range logs {
		logs[i].PetID = petID
		if userId != "" {
			uid := userId
			logs[i].CreatedBy = &uid
		}
		if userName != "" {
			logs[i].CreatedByUsername = &userName
		}
	}

	created, err := h.useCase.CreateBatch(c.Context(), logs)
	if err != nil {
		return apperror.Internal("Failed to batch create litter logs", err)
	}
	return c.Status(fiber.StatusCreated).JSON(created)
}

func (h *LitterHandler) Delete(c *fiber.Ctx) error {
	logID, err := uuid.Parse(c.Params("logId"))
	if err != nil {
		return apperror.BadRequest("Invalid litter log ID", err)
	}
	if err := h.useCase.Delete(c.Context(), logID); err != nil {
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

// MasterDataHandler handles master data endpoints.
type MasterDataHandler struct {
	useCase port.MasterDataUseCase
}

func NewMasterDataHandler(uc port.MasterDataUseCase) *MasterDataHandler {
	return &MasterDataHandler{useCase: uc}
}

func (h *MasterDataHandler) GetCatBreeds(c *fiber.Ctx) error {
	return c.JSON(h.useCase.GetCatBreeds(c.Context()))
}

func (h *MasterDataHandler) GetBloodTypes(c *fiber.Ctx) error {
	return c.JSON(h.useCase.GetBloodTypes(c.Context()))
}

func (h *MasterDataHandler) RegisterRoutes(r fiber.Router) {
	r.Get("/master-data/cat-breeds", h.GetCatBreeds)
	r.Get("/master-data/blood-types", h.GetBloodTypes)
}
