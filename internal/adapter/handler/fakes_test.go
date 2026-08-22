package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/pkg/middleware"
)

// errBoom เป็น error ทั่วไปที่ไม่ใช่ sentinel — ใช้ทดสอบ path 500
var errBoom = errors.New("boom")

// newTestApp สร้าง fiber app ที่มี ErrorHandler ตัวจริง และ inject locals
// เลียนแบบสิ่งที่ auth middleware ทำ เพื่อทดสอบ handler แยกจาก JWT
func newTestApp(locals map[string]any) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: middleware.ErrorHandler})
	app.Use(func(c *fiber.Ctx) error {
		for k, v := range locals {
			c.Locals(k, v)
		}
		return c.Next()
	})
	return app
}

// --- fake PetUseCase ---

type fakePetUC struct {
	allForUser []domain.Pet
	all        []domain.Pet
	one        *domain.Pet
	created    *domain.Pet
	updated    *domain.Pet
	err        error

	createdArg *domain.Pet
	ownerArg   uuid.UUID
	updateArg  *domain.Pet
	deletedID  uuid.UUID
}

func (f *fakePetUC) GetAllForUser(_ context.Context, _ uuid.UUID) ([]domain.Pet, error) {
	return f.allForUser, f.err
}
func (f *fakePetUC) GetAll(_ context.Context) ([]domain.Pet, error) { return f.all, f.err }
func (f *fakePetUC) GetByID(_ context.Context, _ uuid.UUID) (*domain.Pet, error) {
	return f.one, f.err
}
func (f *fakePetUC) Create(_ context.Context, p *domain.Pet, owner uuid.UUID) (*domain.Pet, error) {
	f.createdArg, f.ownerArg = p, owner
	return f.created, f.err
}
func (f *fakePetUC) Update(_ context.Context, _ uuid.UUID, p *domain.Pet) (*domain.Pet, error) {
	f.updateArg = p
	return f.updated, f.err
}
func (f *fakePetUC) Delete(_ context.Context, id uuid.UUID) error {
	f.deletedID = id
	return f.err
}

// --- fake CaregiverUseCase ---

type fakeCaregiverUC struct {
	list    []domain.PetCaregiver
	one     *domain.PetCaregiver
	err     error
	permArg []domain.PetPermission
}

func (f *fakeCaregiverUC) GetForPet(_ context.Context, _ uuid.UUID) ([]domain.PetCaregiver, error) {
	return f.list, f.err
}
func (f *fakeCaregiverUC) Add(_ context.Context, _, _ uuid.UUID) (*domain.PetCaregiver, error) {
	return f.one, f.err
}
func (f *fakeCaregiverUC) UpdatePermissions(_ context.Context, _ uuid.UUID, p []domain.PetPermission) (*domain.PetCaregiver, error) {
	f.permArg = p
	return f.one, f.err
}
func (f *fakeCaregiverUC) Remove(_ context.Context, _ uuid.UUID) error { return f.err }

// --- fake LitterUseCase ---

type fakeLitterUC struct {
	list      []domain.LitterLog
	one       *domain.LitterLog
	batch     []domain.LitterLog
	err       error
	createArg *domain.LitterLog
	batchArg  []domain.LitterLog
}

func (f *fakeLitterUC) GetForPet(_ context.Context, _ uuid.UUID) ([]domain.LitterLog, error) {
	return f.list, f.err
}
func (f *fakeLitterUC) Create(_ context.Context, l *domain.LitterLog) (*domain.LitterLog, error) {
	f.createArg = l
	return f.one, f.err
}
func (f *fakeLitterUC) CreateBatch(_ context.Context, l []domain.LitterLog) ([]domain.LitterLog, error) {
	f.batchArg = l
	return f.batch, f.err
}
func (f *fakeLitterUC) Delete(_ context.Context, _ uuid.UUID) error { return f.err }

// --- fake WaterUseCase ---

type fakeWaterUC struct {
	list      []domain.WaterLog
	one       *domain.WaterLog
	err       error
	createArg *domain.WaterLog
}

func (f *fakeWaterUC) Create(_ context.Context, l *domain.WaterLog) (*domain.WaterLog, error) {
	f.createArg = l
	return f.one, f.err
}
func (f *fakeWaterUC) GetByPetID(_ context.Context, _ uuid.UUID) ([]domain.WaterLog, error) {
	return f.list, f.err
}
func (f *fakeWaterUC) Delete(_ context.Context, _ uuid.UUID) error { return f.err }
