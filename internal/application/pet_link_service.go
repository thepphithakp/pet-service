package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
)

// PetLinkService implements port.PetLinkUseCase.
//
// แยกจาก PetService/Authorizer เพราะ "สองคนนี้เกี่ยวข้องกับสัตว์เลี้ยงตัวนี้
// ทั้งคู่ไหม" ไม่ใช่คำถามแบบ "actor คนนี้มีสิทธิ์เข้าถึง resource นี้ไหม" —
// ไม่มี actor เดี่ยวจาก context ให้ authz.Authorize ตรวจ มีแต่สอง userID ที่
// ถูกส่งมาเปรียบเทียบกัน (เรียกจาก chat-service ผ่าน service token)
type PetLinkService struct {
	pets port.PetRepository
}

func NewPetLinkService(pets port.PetRepository) *PetLinkService {
	return &PetLinkService{pets: pets}
}

// IsLinked ตอบว่าผู้ใช้สองคนนี้เกี่ยวข้องกับสัตว์เลี้ยงตัวนี้ทั้งคู่ไหม
// (เจ้าของหรือผู้ดูแลก็นับ) — ย้ายมาจาก handler เดิมที่เรียก FindAccess
// ตรงๆ สองครั้งแล้วเทียบเอง เพื่อให้ business rule นี้มีที่อยู่เดียว ไม่ใช่
// ซ่อนอยู่ใน handler ซึ่ง caller ในอนาคต (gRPC, cron) จะมองไม่เห็นเลย
func (s *PetLinkService) IsLinked(ctx context.Context, petID, userA, userB uuid.UUID) (bool, error) {
	accessA, err := s.pets.FindAccess(ctx, petID, userA)
	if err != nil {
		return false, err
	}
	accessB, err := s.pets.FindAccess(ctx, petID, userB)
	if err != nil {
		return false, err
	}
	return accessA.Level != domain.AccessNone && accessB.Level != domain.AccessNone, nil
}
