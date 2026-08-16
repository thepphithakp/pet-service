package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
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

// petRequest is the HTTP request/response DTO for pets.
type petRequest struct {
	Name             string   `json:"name"`
	Species          string   `json:"species"`
	Breed            string   `json:"breed"`
	ColorCode        string   `json:"colorCode"`
	BirthDate        string   `json:"birthDate"`
	Gender           string   `json:"gender"`
	AvatarData       []byte   `json:"avatarData,omitempty"`
	CurrentWeight    *float64 `json:"currentWeight"`
	MicrochipId      *string  `json:"microchipId"`
	IsSpayedNeutered bool     `json:"isSpayedNeutered"`
	BloodType        *string  `json:"bloodType"`
	Allergies        *string  `json:"allergies"`
	Personality      *string  `json:"personality"`
}

func (h *PetHandler) GetAll(c *fiber.Ctx) error {
	userIDStr, ok := c.Locals("userId").(string)
	if !ok || userIDStr == "" {
		return apperror.Unauthorized("Missing user ID in token")
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return apperror.Unauthorized("Invalid user ID in token")
	}

	pets, err := h.useCase.GetAllForUser(c.Context(), userID)
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.JSON(pets)
}

func (h *PetHandler) AdminGetAll(c *fiber.Ctx) error {
	pets, err := h.useCase.GetAll(c.Context())
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
	pet, err := h.useCase.GetByID(c.Context(), id)
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.JSON(pet)
}

func (h *PetHandler) Create(c *fiber.Ctx) error {
	userIDStr := c.Locals("userId").(string)
	ownerID, err := uuid.Parse(userIDStr)
	if err != nil {
		return apperror.Unauthorized("Invalid user ID in token")
	}

	userName, _ := c.Locals("userName").(string)

	var pet domain.Pet
	if err := c.BodyParser(&pet); err != nil {
		return apperror.BadRequest("Invalid request body", err)
	}
	pet.OwnerUsername = userName

	created, err := h.useCase.Create(c.Context(), &pet, ownerID)
	if err != nil {
		return apperror.Internal("Failed to create pet", err)
	}
	return c.Status(fiber.StatusCreated).JSON(created)
}

func (h *PetHandler) Update(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.BadRequest("Invalid pet ID", err)
	}

	var pet domain.Pet
	if err := c.BodyParser(&pet); err != nil {
		return apperror.BadRequest("Invalid request body", err)
	}

	updated, err := h.useCase.Update(c.Context(), id, &pet)
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
	if err := h.useCase.Delete(c.Context(), id); err != nil {
		return apperror.FromDomain(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// RegisterRoutes wires this handler's routes onto the given router group.
func (h *PetHandler) RegisterRoutes(router fiber.Router) {
	router.Get("/pets", h.GetAll)
	router.Get("/admin/pets", h.AdminGetAll)
	router.Get("/pets/:id", h.GetOne)
	router.Post("/pets", h.Create)
	router.Put("/pets/:id", h.Update)
	router.Delete("/pets/:id", h.Delete)
}
