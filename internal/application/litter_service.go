package application

import (
	"context"

	"github.com/google/uuid"
	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
)

// LitterService implements port.LitterUseCase.
type LitterService struct {
	repo port.LitterRepository
	eventPublisher port.EventPublisher
}

func NewLitterService(repo port.LitterRepository, eventPublisher port.EventPublisher) *LitterService {
	return &LitterService{
		repo: repo,
		eventPublisher: eventPublisher,
	}
}

func (s *LitterService) GetForPet(ctx context.Context, petID uuid.UUID) ([]domain.LitterLog, error) {
	return s.repo.FindByPetID(ctx, petID)
}

func (s *LitterService) Create(ctx context.Context, log *domain.LitterLog) (*domain.LitterLog, error) {
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
			EntityType: "Pet",
			Payload: map[string]interface{}{
				"type":   log.Type,
				"amount": log.Amount,
			},
		})
	}
	return created, err
}

func (s *LitterService) CreateBatch(ctx context.Context, logs []domain.LitterLog) ([]domain.LitterLog, error) {
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
				EntityType: "Pet",
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

func (s *LitterService) Delete(ctx context.Context, logID uuid.UUID) error {
	return s.repo.Delete(ctx, logID)
}

// MasterDataService implements port.MasterDataUseCase.
type MasterDataService struct{}

func NewMasterDataService() *MasterDataService { return &MasterDataService{} }

func (s *MasterDataService) GetCatBreeds(_ context.Context) []string {
	return []string{
		"Scottish Fold (หูพับ)", "Scottish Straight (หูตั้ง)", "British Shorthair",
		"Persian", "Maine Coon", "Siamese (วิเชียรมาศ)", "Khao Manee (ขาวมณี)",
		"Sphynx", "Bengal", "Ragdoll", "American Shorthair",
		"Exotic Shorthair", "Munchkin (ขาสั้น)", "Mixed / Other (พันธุ์ผสม/อื่นๆ)",
	}
}

func (s *MasterDataService) GetBloodTypes(_ context.Context) []string {
	return []string{"Unknown", "A", "B", "AB"}
}
