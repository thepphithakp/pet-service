package application

import (
	"context"

	"github.com/google/uuid"
	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
)

// PetService implements port.PetUseCase.
type PetService struct {
	repo port.PetRepository
	eventPublisher port.EventPublisher
}

// NewPetService creates a new PetService.
func NewPetService(repo port.PetRepository, eventPublisher port.EventPublisher) *PetService {
	return &PetService{
		repo: repo,
		eventPublisher: eventPublisher,
	}
}

func (s *PetService) GetAllForUser(ctx context.Context, userID uuid.UUID) ([]domain.Pet, error) {
    return s.repo.FindAllForUser(ctx, userID)
}

func (s *PetService) GetAll(ctx context.Context) ([]domain.Pet, error) {
	return s.repo.FindAll(ctx)
}

func (s *PetService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Pet, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *PetService) Create(ctx context.Context, pet *domain.Pet, ownerID uuid.UUID) (*domain.Pet, error) {
	pet.ID = uuid.New()
	pet.OwnerID = ownerID
	
	created, err := s.repo.Save(ctx, pet)
	if err == nil && s.eventPublisher != nil {
		s.eventPublisher.Publish(ctx, port.EventLog{
			EventType:  "PetProfile",
			Action:     "Pet Created",
			ActorID:    ownerID.String(),
			EntityID:   pet.ID.String(),
			EntityType: "Pet",
			Payload: map[string]interface{}{
				"name":    pet.Name,
				"species": pet.Species,
			},
		})
	}
	return created, err
}

func (s *PetService) Update(ctx context.Context, id uuid.UUID, incoming *domain.Pet) (*domain.Pet, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// Patch existing entity with incoming changes
	if incoming.Name != "" {
		existing.Name = incoming.Name
	}
	if incoming.Species != "" {
		existing.Species = incoming.Species
	}
	if incoming.Breed != "" {
		existing.Breed = incoming.Breed
	}
	if incoming.ColorCode != "" {
		existing.ColorCode = incoming.ColorCode
	}
	if !incoming.BirthDate.IsZero() {
		existing.BirthDate = incoming.BirthDate
	}
	if incoming.Gender != "" {
		existing.Gender = incoming.Gender
	}
	if len(incoming.AvatarData) > 0 {
		existing.AvatarData = incoming.AvatarData
	}
	if incoming.CurrentWeight != nil {
		existing.CurrentWeight = incoming.CurrentWeight
	}
	if incoming.MicrochipId != nil {
		existing.MicrochipId = incoming.MicrochipId
	}
	existing.IsSpayedNeutered = incoming.IsSpayedNeutered
	if incoming.BloodType != nil {
		existing.BloodType = incoming.BloodType
	}
	if incoming.Allergies != nil {
		existing.Allergies = incoming.Allergies
	}
	if incoming.Personality != nil {
		existing.Personality = incoming.Personality
	}

	updatedPet, err := s.repo.Update(ctx, existing)
	if err == nil && s.eventPublisher != nil {
		actorID := updatedPet.OwnerUsername
		s.eventPublisher.Publish(ctx, port.EventLog{
			EventType:  "PetProfile",
			Action:     "Pet Updated",
			ActorID:    actorID,
			EntityID:   updatedPet.ID.String(),
			EntityType: "Pet",
			Payload: map[string]interface{}{
				"name": updatedPet.Name,
			},
		})
	}
	return updatedPet, err
}

func (s *PetService) Delete(ctx context.Context, id uuid.UUID) error {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	err = s.repo.Delete(ctx, id)
	if err == nil && s.eventPublisher != nil {
		actorID := existing.OwnerUsername
		s.eventPublisher.Publish(ctx, port.EventLog{
			EventType:  "PetProfile",
			Action:     "Pet Deleted",
			ActorID:    actorID,
			EntityID:   id.String(),
			EntityType: "Pet",
			Payload: map[string]interface{}{
				"name": existing.Name,
			},
		})
	}
	return err
}
