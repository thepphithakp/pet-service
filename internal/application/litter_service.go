package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
)

// LitterService implements port.LitterUseCase.
type LitterService struct {
	repo   port.LitterRepository
	events *EventRecorder
	authz  *Authorizer
}

func NewLitterService(repo port.LitterRepository, events *EventRecorder, authz *Authorizer) *LitterService {
	return &LitterService{
		repo:   repo,
		events: events,
		authz:  authz,
	}
}

func (s *LitterService) GetForPet(ctx context.Context, petID uuid.UUID) ([]domain.LitterLog, error) {
	if err := s.authz.Authorize(ctx, petID, ReqLogRead); err != nil {
		return nil, err
	}
	return s.repo.FindByPetID(ctx, petID)
}

// GetPageForPet คืนหนึ่งหน้า — ตรวจสิทธิ์ชุดเดียวกับการอ่านทั้งหมด
func (s *LitterService) GetPageForPet(ctx context.Context, petID uuid.UUID, page domain.LogPage) ([]domain.LitterLog, bool, error) {
	if err := s.authz.Authorize(ctx, petID, ReqLogRead); err != nil {
		return nil, false, err
	}
	return s.repo.FindPageByPetID(ctx, petID, page)
}

func (s *LitterService) Create(ctx context.Context, log *domain.LitterLog) (*domain.LitterLog, error) {
	if err := s.authz.Authorize(ctx, log.PetID, ReqLitterWrite); err != nil {
		return nil, err
	}
	// ⚠️ ต้องเช็ค Nil ก่อน ห้ามเขียนทับ
	//
	// แอปสร้าง UUID เองแล้วแสดงรายการทันทีก่อน POST จะกลับมา
	// ถ้าเขียนทับ พอ refresh จะได้อีกแถวที่ id คนละตัว แอปเลยแสดงสองรายการ
	// จากการบันทึกครั้งเดียว และกดลบรายการของตัวเองจะได้ 404
	// (อาการเดียวกับที่เจอกับ water log เมื่อ 2026-08-23 — ตอนนั้นแก้แค่ water
	//  แต่ litter ยังเขียนทับอยู่)
	if log.ID == uuid.Nil {
		log.ID = uuid.New()
	}

	var created *domain.LitterLog
	err := s.events.Record(ctx, func(txCtx context.Context) ([]port.EventLog, error) {
		var err error
		created, err = s.repo.Save(txCtx, log)
		if err != nil {
			return nil, err
		}

		actorID, actorUsername := actorFrom(log.CreatedBy, log.CreatedByUsername)
		return []port.EventLog{{
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
		}}, nil
	})
	if err != nil {
		return nil, err
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
	var createdLogs []domain.LitterLog
	err := s.events.Record(ctx, func(txCtx context.Context) ([]port.EventLog, error) {
		var err error
		createdLogs, err = s.repo.SaveBatch(txCtx, logs)
		if err != nil {
			return nil, err
		}

		events := make([]port.EventLog, 0, len(createdLogs))
		for _, log := range createdLogs {
			actorID, actorUsername := actorFrom(log.CreatedBy, log.CreatedByUsername)
			events = append(events, port.EventLog{
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
		return events, nil
	})
	if err != nil {
		return nil, err
	}
	return createdLogs, nil
}

func (s *LitterService) Delete(ctx context.Context, petID, logID uuid.UUID) error {
	if err := s.authz.Authorize(ctx, petID, ReqLitterWrite); err != nil {
		return err
	}
	return s.repo.Delete(ctx, petID, logID)
}
