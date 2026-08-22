package application

import (
	"context"

	"github.com/google/uuid"
	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
)

// LitterService implements port.LitterUseCase.
type LitterService struct {
	repo           port.LitterRepository
	eventPublisher port.EventPublisher
	authz          *Authorizer
}

func NewLitterService(repo port.LitterRepository, eventPublisher port.EventPublisher, authz *Authorizer) *LitterService {
	return &LitterService{
		repo:           repo,
		eventPublisher: eventPublisher,
		authz:          authz,
	}
}

func (s *LitterService) GetForPet(ctx context.Context, petID uuid.UUID) ([]domain.LitterLog, error) {
	if err := s.authz.Authorize(ctx, petID, ReqLogRead); err != nil {
		return nil, err
	}
	return s.repo.FindByPetID(ctx, petID)
}

func (s *LitterService) Create(ctx context.Context, log *domain.LitterLog) (*domain.LitterLog, error) {
	if err := s.authz.Authorize(ctx, log.PetID, ReqLitterWrite); err != nil {
		return nil, err
	}
	log.ID = uuid.New()

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
			EventType:     "LitterLog",
			Action:        "Litter Log Added",
			ActorID:       actorID,
			ActorUsername: actorUsername,
			EntityID:      log.PetID.String(),
			EntityType:    "Pet",
			Payload: map[string]interface{}{
				"type":   log.Type,
				"amount": log.Amount,
			},
		})
	}
	return created, err
}

func (s *LitterService) CreateBatch(ctx context.Context, logs []domain.LitterLog) ([]domain.LitterLog, error) {
	// C-7: gorm.Create กับ slice ว่างคืน ErrEmptySlice → 500 ปิดไว้ตั้งแต่ต้นทาง
	if len(logs) == 0 {
		return []domain.LitterLog{}, nil
	}
	// ทุกแถวต้องเป็นของสัตว์เลี้ยงตัวเดียวกัน (handler เซ็ตจาก path ให้แล้ว)
	// ตรวจอีกชั้นเผื่อมี caller อื่นเรียกตรงๆ
	petID := logs[0].PetID
	for i := range logs {
		if logs[i].PetID != petID {
			return nil, domain.ErrForbidden
		}
	}
	if err := s.authz.Authorize(ctx, petID, ReqLitterWrite); err != nil {
		return nil, err
	}
	for i := range logs {
		if logs[i].ID == uuid.Nil {
			logs[i].ID = uuid.New()
		}
	}
	createdLogs, err := s.repo.SaveBatch(ctx, logs)
	if err == nil && s.eventPublisher != nil {
		for _, log := range createdLogs {
			actorID := ""
			actorUsername := ""
			if log.CreatedBy != nil {
				actorID = *log.CreatedBy
			}
			if log.CreatedByUsername != nil {
				actorUsername = *log.CreatedByUsername
			}
			s.eventPublisher.Publish(ctx, port.EventLog{
				EventType:     "LitterLog",
				Action:        "Litter Log Added",
				ActorID:       actorID,
				ActorUsername: actorUsername,
				EntityID:      log.PetID.String(),
				EntityType:    "Pet",
				Payload: map[string]interface{}{
					"type":   log.Type,
					"amount": log.Amount,
					"batch":  true,
				},
			})
		}
	}
	return createdLogs, err
}

func (s *LitterService) Delete(ctx context.Context, petID, logID uuid.UUID) error {
	if err := s.authz.Authorize(ctx, petID, ReqLitterWrite); err != nil {
		return err
	}
	return s.repo.Delete(ctx, petID, logID)
}
