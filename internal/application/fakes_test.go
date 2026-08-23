package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
)

var errBoom = errors.New("boom")

type fakePetRepo struct {
	byID     *domain.Pet
	findErr  error
	saved    *domain.Pet
	updated  *domain.Pet
	saveErr  error
	deleteID uuid.UUID

	access    domain.PetAccess
	accessErr error

	summaries []domain.PetSummary
	avatar    *domain.Avatar
	avatarErr error
}

func (f *fakePetRepo) FindAllForUserSummary(context.Context, uuid.UUID) ([]domain.PetSummary, error) {
	return f.summaries, f.findErr
}

func (f *fakePetRepo) FindAllSummary(context.Context) ([]domain.PetSummary, error) {
	return f.summaries, f.findErr
}

func (f *fakePetRepo) FindAvatar(context.Context, uuid.UUID) (*domain.Avatar, error) {
	return f.avatar, f.avatarErr
}

func (f *fakePetRepo) FindAccess(context.Context, uuid.UUID, uuid.UUID) (domain.PetAccess, error) {
	return f.access, f.accessErr
}

// fakeCaps จำลอง role → capability
type fakeCaps struct {
	table map[string][]string
	err   error
}

func (f *fakeCaps) HasAny(_ context.Context, roles []string, caps ...string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	for _, r := range roles {
		for _, have := range f.table[r] {
			for _, want := range caps {
				if have == want {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// ctxAs สร้าง context ที่มี actor พร้อม role ตามต้องการ
func ctxAs(userID uuid.UUID, roles ...string) context.Context {
	if len(roles) == 0 {
		roles = []string{domain.RoleUser}
	}
	return domain.WithActor(context.Background(), domain.Actor{
		UserID:   userID,
		Username: "tester",
		Roles:    roles,
	})
}

// adminCaps คือ capability ของ SUPER_ADMIN ตามที่ R__0005 seed ไว้
func adminCaps() *fakeCaps {
	return &fakeCaps{table: map[string][]string{
		domain.RoleSuperAdmin: {
			domain.CapPetReadAny, domain.CapPetWriteAny, domain.CapPetDeleteAny,
			domain.CapCaregiverManageAny, domain.CapLogReadAny, domain.CapLogWriteAny,
			domain.CapMasterDataWrite,
		},
	}}
}

func (f *fakePetRepo) FindAllForUser(context.Context, uuid.UUID) ([]domain.Pet, error) {
	return nil, f.findErr
}
func (f *fakePetRepo) FindAll(context.Context) ([]domain.Pet, error) { return nil, f.findErr }
func (f *fakePetRepo) FindByID(context.Context, uuid.UUID) (*domain.Pet, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	cp := *f.byID
	return &cp, nil
}
func (f *fakePetRepo) Save(_ context.Context, p *domain.Pet) (*domain.Pet, error) {
	f.saved = p
	return p, f.saveErr
}
func (f *fakePetRepo) Update(_ context.Context, p *domain.Pet) (*domain.Pet, error) {
	f.updated = p
	return p, f.saveErr
}
func (f *fakePetRepo) Delete(_ context.Context, id uuid.UUID) error {
	f.deleteID = id
	return f.saveErr
}

type fakePublisher struct{ events []port.EventLog }

func (f *fakePublisher) Publish(_ context.Context, e port.EventLog) {
	f.events = append(f.events, e)
}

func strp(s string) *string { return &s }
