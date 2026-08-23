package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
)

// PetService implements port.PetUseCase.
type PetService struct {
	repo   port.PetRepository
	events *EventRecorder
	authz  *Authorizer
}

// NewPetService creates a new PetService.
func NewPetService(repo port.PetRepository, events *EventRecorder, authz *Authorizer) *PetService {
	return &PetService{
		repo:   repo,
		events: events,
		authz:  authz,
	}
}

func (s *PetService) GetAllForUser(ctx context.Context, userID uuid.UUID) ([]domain.Pet, error) {
	return s.repo.FindAllForUser(ctx, userID)
}

// GetAll ดึงสัตว์เลี้ยงทั้งระบบ — เฉพาะ admin เท่านั้น
//
// เดิม endpoint นี้ไม่มีการตรวจสิทธิ์เลย token ที่ valid ตัวไหนก็ดึงข้อมูลทั้งระบบได้ (S-2)
func (s *PetService) GetAll(ctx context.Context) ([]domain.Pet, error) {
	if err := s.authz.AuthorizeGlobal(ctx, domain.CapPetReadAny); err != nil {
		return nil, err
	}
	return s.repo.FindAll(ctx)
}

// GetAllForUserSummary คืนรายการที่ไม่มี avatar
//
// authorization เท่ากับ GetAllForUser — repository กรองด้วย owner_id
// และ pet_caregivers อยู่แล้ว จึงไม่ต้องเรียก authz ซ้ำ
func (s *PetService) GetAllForUserSummary(ctx context.Context, userID uuid.UUID) ([]domain.PetSummary, error) {
	return s.repo.FindAllForUserSummary(ctx, userID)
}

// GetAllSummary ใช้กับ endpoint ของ admin ซึ่งคุมสิทธิ์ที่ชั้น route แล้ว
func (s *PetService) GetAllSummary(ctx context.Context) ([]domain.PetSummary, error) {
	return s.repo.FindAllSummary(ctx)
}

// GetAvatar ตรวจสิทธิ์ก่อนคืนรูป
//
// ใช้สิทธิ์ชุดเดียวกับการอ่านข้อมูลสัตว์เลี้ยง — ใครดูข้อมูลได้ก็ดูรูปได้
// ถ้าไม่ตรวจ ใครก็ดึงรูปสัตว์เลี้ยงของคนอื่นได้ถ้ารู้ UUID
func (s *PetService) GetAvatar(ctx context.Context, petID uuid.UUID) (*domain.Avatar, error) {
	if err := s.authz.Authorize(ctx, petID, ReqPetRead); err != nil {
		return nil, err
	}
	return s.repo.FindAvatar(ctx, petID)
}

func (s *PetService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Pet, error) {
	if err := s.authz.Authorize(ctx, id, ReqPetRead); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

func (s *PetService) Create(ctx context.Context, pet *domain.Pet, ownerID uuid.UUID) (*domain.Pet, error) {
	pet.ID = uuid.New()
	pet.OwnerID = ownerID

	var created *domain.Pet
	err := s.events.Record(ctx, func(txCtx context.Context) ([]port.EventLog, error) {
		var err error
		created, err = s.repo.Save(txCtx, pet)
		if err != nil {
			return nil, err
		}

		actor, _ := domain.ActorFromContext(txCtx)
		return []port.EventLog{{
			EventType:     "PetProfile",
			Action:        "Pet Created",
			ActorID:       ownerID.String(),
			ActorUsername: actor.Username,
			EntityID:      pet.ID.String(),
			EntityType:    "Pet",
			Payload: map[string]interface{}{
				"name":    pet.Name,
				"species": pet.Species,
			},
		}}, nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *PetService) Update(ctx context.Context, id uuid.UUID, incoming *domain.Pet) (*domain.Pet, error) {
	if err := s.authz.Authorize(ctx, id, ReqPetUpdate); err != nil {
		return nil, err
	}
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

	if actor, ok := domain.ActorFromContext(ctx); ok {
		uid := actor.UserID.String()
		existing.UpdatedBy = &uid
	}

	var updatedPet *domain.Pet
	err = s.events.Record(ctx, func(txCtx context.Context) ([]port.EventLog, error) {
		var err error
		updatedPet, err = s.repo.Update(txCtx, existing)
		if err != nil {
			return nil, err
		}

		// C-2: เดิมใส่ OwnerUsername (username ของ "เจ้าของ") ลงในช่อง ActorID
		// ทำให้ audit trail บอกไม่ได้ว่าใครเป็นคนแก้จริง
		actor, _ := domain.ActorFromContext(txCtx)
		return []port.EventLog{{
			EventType:     "PetProfile",
			Action:        "Pet Updated",
			ActorID:       actor.UserID.String(),
			ActorUsername: actor.Username,
			EntityID:      updatedPet.ID.String(),
			EntityType:    "Pet",
			Payload: map[string]interface{}{
				"name": updatedPet.Name,
			},
		}}, nil
	})
	if err != nil {
		return nil, err
	}
	return updatedPet, nil
}

func (s *PetService) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.authz.Authorize(ctx, id, ReqPetDelete); err != nil {
		return err
	}
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	err = s.events.Record(ctx, func(txCtx context.Context) ([]port.EventLog, error) {
		if err := s.repo.Delete(txCtx, id); err != nil {
			return nil, err
		}

		actor, _ := domain.ActorFromContext(txCtx)
		return []port.EventLog{{
			EventType:     "PetProfile",
			Action:        "Pet Deleted",
			ActorID:       actor.UserID.String(),
			ActorUsername: actor.Username,
			EntityID:      id.String(),
			EntityType:    "Pet",
			Payload: map[string]interface{}{
				"name": existing.Name,
			},
		}}, nil
	})
	return err
}
