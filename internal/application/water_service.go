package application

import (
	"context"

	"github.com/google/uuid"
	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
)

type WaterService struct {
	repo           port.WaterRepository
	eventPublisher port.EventPublisher
}

func NewWaterService(repo port.WaterRepository, eventPublisher port.EventPublisher) *WaterService {
	return &WaterService{repo: repo, eventPublisher: eventPublisher}
}

func (s *WaterService) Create(ctx context.Context, log *domain.WaterLog) (*domain.WaterLog, error) {
	if log.ID == uuid.Nil {
		log.ID = uuid.New()
	}
	created, err := s.repo.Save(ctx, log)
	if err == nil && s.eventPublisher != nil {
		actorID := ""
		actorUsername := ""
		if log.CreatedBy != nil {
			actorID = *log.CreatedBy
		}
		if log.CreatedByUsername != nil {
			actorUsername = *log.CreatedByUsername
		}
		s.eventPublisher.Publish(ctx, port.EventLog{
			EventType:     "WaterLog",
			Action:        "Water Intake Logged",
			ActorID:       actorID,
			ActorUsername: actorUsername,
			EntityID:      log.PetID.String(),
			EntityType:    "Pet",
			Payload: map[string]interface{}{
				"amount": log.Amount,
			},
		})
	}
	return created, err
}

func (s *WaterService) GetByPetID(ctx context.Context, petID uuid.UUID) ([]domain.WaterLog, error) {
	return s.repo.FindByPetID(ctx, petID)
}

func (s *WaterService) Delete(ctx context.Context, logID uuid.UUID) error {
	return s.repo.Delete(ctx, logID)
}
