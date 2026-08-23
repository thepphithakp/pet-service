package port

import (
	"context"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
)

type WaterRepository interface {
	Save(ctx context.Context, log *domain.WaterLog) (*domain.WaterLog, error)
	FindByPetID(ctx context.Context, petID uuid.UUID) ([]domain.WaterLog, error)

	// FindPageByPetID คืนหนึ่งหน้าตามลำดับ (date desc, id desc)
	//
	// ใช้ keyset ไม่ใช่ offset — offset จะข้ามหรือซ้ำรายการเมื่อมี log ใหม่
	// เพิ่มเข้ามาระหว่างที่ผู้ใช้กำลังเลื่อนดู ซึ่งเกิดตลอดกับรายการที่เรียงจากใหม่ไปเก่า
	//
	// คืน hasMore แยกต่างหาก เพราะการนับทั้งตารางเพื่อบอกว่าเหลืออีกไหม
	// แพงกว่าการดึงมาเกินหนึ่งแถวแล้วดูว่าได้ครบไหม
	FindPageByPetID(ctx context.Context, petID uuid.UUID, page domain.LogPage) (logs []domain.WaterLog, hasMore bool, err error)
	Delete(ctx context.Context, petID, logID uuid.UUID) error
}

type WaterUseCase interface {
	Create(ctx context.Context, log *domain.WaterLog) (*domain.WaterLog, error)
	GetByPetID(ctx context.Context, petID uuid.UUID) ([]domain.WaterLog, error)
	GetPageByPetID(ctx context.Context, petID uuid.UUID, page domain.LogPage) (logs []domain.WaterLog, hasMore bool, err error)
	Delete(ctx context.Context, petID, logID uuid.UUID) error
}
