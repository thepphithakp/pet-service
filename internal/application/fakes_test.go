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
