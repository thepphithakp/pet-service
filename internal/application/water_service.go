package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
)

type WaterService struct {
	repo   port.WaterRepository
	events *EventRecorder
	authz  *Authorizer
}

func NewWaterService(repo port.WaterRepository, events *EventRecorder, authz *Authorizer) *WaterService {
	return &WaterService{repo: repo, events: events, authz: authz}
}

func (s *WaterService) Create(ctx context.Context, log *domain.WaterLog) (*domain.WaterLog, error) {
	if err := s.authz.Authorize(ctx, log.PetID, ReqWaterWrite); err != nil {
		return nil, err
	}
	if log.ID == uuid.Nil {
		log.ID = uuid.New()
	}
	var created *domain.WaterLog
	err := s.events.Record(ctx, func(txCtx context.Context) ([]port.EventLog, error) {
		var err error
		created, err = s.repo.Save(txCtx, log)
		if err != nil {
			return nil, err
		}

		actorID, actorUsername := actorFrom(log.CreatedBy, log.CreatedByUsername)
		return []port.EventLog{{
			EventType:     "WaterLog",
			Action:        "Water Intake Logged",
			ActorID:       actorID,
			ActorUsername: actorUsername,
			EntityID:      log.PetID.String(),
			EntityType:    "Pet",
			Payload: map[string]interface{}{
				"amount": log.Amount,
			},
		}}, nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *WaterService) GetByPetID(ctx context.Context, petID uuid.UUID) ([]domain.WaterLog, error) {
	if err := s.authz.Authorize(ctx, petID, ReqLogRead); err != nil {
		return nil, err
	}
	return s.repo.FindByPetID(ctx, petID)
}

// GetPageByPetID คืนหนึ่งหน้า — ตรวจสิทธิ์ชุดเดียวกับการอ่านทั้งหมด
func (s *WaterService) GetPageByPetID(ctx context.Context, petID uuid.UUID, page domain.LogPage) ([]domain.WaterLog, bool, error) {
	if err := s.authz.Authorize(ctx, petID, ReqLogRead); err != nil {
		return nil, false, err
	}
	return s.repo.FindPageByPetID(ctx, petID, page)
}

func (s *WaterService) Delete(ctx context.Context, petID, logID uuid.UUID) error {
	if err := s.authz.Authorize(ctx, petID, ReqWaterWrite); err != nil {
		return err
	}
	return s.repo.Delete(ctx, petID, logID)
}
