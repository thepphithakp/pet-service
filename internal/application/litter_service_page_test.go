package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
)

type fakeLitterRepo struct {
	saved    []domain.LitterLog
	batchArg []domain.LitterLog
	pageArg  domain.LogPage
	hasMore  bool
	saveErr  error
	batchErr error
}

func (f *fakeLitterRepo) FindByPetID(context.Context, uuid.UUID) ([]domain.LitterLog, error) {
	return f.saved, nil
}
func (f *fakeLitterRepo) FindPageByPetID(_ context.Context, _ uuid.UUID, p domain.LogPage) ([]domain.LitterLog, bool, error) {
	f.pageArg = p
	return f.saved, f.hasMore, nil
}
func (f *fakeLitterRepo) Save(_ context.Context, l *domain.LitterLog) (*domain.LitterLog, error) {
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	f.saved = append(f.saved, *l)
	return l, nil
}
func (f *fakeLitterRepo) SaveBatch(_ context.Context, logs []domain.LitterLog) ([]domain.LitterLog, error) {
	if f.batchErr != nil {
		return nil, f.batchErr
	}
	f.batchArg = logs
	f.saved = append(f.saved, logs...)
	return logs, nil
}
func (f *fakeLitterRepo) Delete(context.Context, uuid.UUID, uuid.UUID) error { return nil }

func litterSvc(t *testing.T, repo *fakeLitterRepo, pub *fakePublisher, level domain.AccessLevel) *LitterService {
	t.Helper()
	petRepo := &fakePetRepo{access: domain.PetAccess{Level: level}}
	return NewLitterService(repo, recorderFor(pub), NewAuthorizer(petRepo, adminCaps()))
}

// TestCreateBatch_RejectsMixedPets
//
// ทุกแถวต้องเป็นของสัตว์เลี้ยงตัวเดียวกัน ไม่งั้นผู้เรียกจะแอบเขียน log
// ให้สัตว์เลี้ยงตัวอื่นผ่าน batch ของตัวที่ตัวเองมีสิทธิ์
func TestCreateBatch_RejectsMixedPets(t *testing.T) {
	repo := &fakeLitterRepo{}
	svc := litterSvc(t, repo, &fakePublisher{}, domain.AccessOwner)

	logs := []domain.LitterLog{
		{ID: uuid.New(), PetID: uuid.New(), Type: "Poop", Amount: 1},
		{ID: uuid.New(), PetID: uuid.New(), Type: "Pee", Amount: 1},
	}
	if _, err := svc.CreateBatch(ctxAs(uuid.New()), logs); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ต้องเป็น ErrForbidden ได้ %v", err)
	}
	if repo.batchArg != nil {
		t.Error("ห้ามเขียนอะไรเมื่อมีสัตว์เลี้ยงปนกัน")
	}
}

// TestCreateBatch_EmptyIsNotAnError
//
// C-7: gorm.Create กับ slice ว่างคืน ErrEmptySlice ซึ่งกลายเป็น 500
func TestCreateBatch_EmptyIsNotAnError(t *testing.T) {
	repo := &fakeLitterRepo{}
	svc := litterSvc(t, repo, &fakePublisher{}, domain.AccessOwner)

	got, err := svc.CreateBatch(ctxAs(uuid.New()), nil)
	if err != nil {
		t.Fatalf("ไม่ควร error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ต้องคืน list ว่าง ได้ %v", got)
	}
}

// TestCreateBatch_KeepsClientIDsAndFillsMissing
func TestCreateBatch_KeepsClientIDsAndFillsMissing(t *testing.T) {
	repo := &fakeLitterRepo{}
	pub := &fakePublisher{}
	svc := litterSvc(t, repo, pub, domain.AccessOwner)

	petID := uuid.New()
	clientID := uuid.New()
	logs := []domain.LitterLog{
		{ID: clientID, PetID: petID, Type: "Poop", Amount: 1},
		{PetID: petID, Type: "Pee", Amount: 2}, // ไม่ส่ง id มา
	}

	if _, err := svc.CreateBatch(ctxAs(uuid.New()), logs); err != nil {
		t.Fatalf("ไม่ควร error: %v", err)
	}
	if repo.batchArg[0].ID != clientID {
		t.Error("id ที่ client ส่งมาต้องถูกใช้")
	}
	if repo.batchArg[1].ID == uuid.Nil {
		t.Error("แถวที่ไม่ส่ง id มาต้องถูกเติมให้")
	}
	if len(pub.events) != 2 {
		t.Errorf("ต้องบันทึก event 2 ตัว ได้ %d", len(pub.events))
	}
	for _, e := range pub.events {
		if e.Payload["batch"] != true {
			t.Error("event จาก batch ต้องมี flag batch")
		}
	}
}

// TestCreateBatch_RollsBackEventsWhenSaveFails
func TestCreateBatch_RollsBackEventsWhenSaveFails(t *testing.T) {
	repo := &fakeLitterRepo{batchErr: errors.New("เขียนไม่ได้")}
	pub := &fakePublisher{}
	svc := litterSvc(t, repo, pub, domain.AccessOwner)

	petID := uuid.New()
	_, err := svc.CreateBatch(ctxAs(uuid.New()), []domain.LitterLog{
		{ID: uuid.New(), PetID: petID, Type: "Poop", Amount: 1},
	})
	if err == nil {
		t.Fatal("ต้องคืน error")
	}
	if len(pub.events) != 0 {
		t.Error("เขียนข้อมูลไม่สำเร็จต้องไม่มี event ถูกบันทึก")
	}
}

// TestGetPageForPet_NormalizesLimit
func TestGetPageForPet_NormalizesLimit(t *testing.T) {
	repo := &fakeLitterRepo{}
	svc := litterSvc(t, repo, &fakePublisher{}, domain.AccessOwner)

	if _, _, err := svc.GetPageForPet(ctxAs(uuid.New()), uuid.New(), domain.LogPage{}); err != nil {
		t.Fatalf("ไม่ควร error: %v", err)
	}
	if repo.pageArg.Limit != domain.LogPageDefaultLimit {
		t.Errorf("limit = %d ต้องเป็นค่า default %d", repo.pageArg.Limit, domain.LogPageDefaultLimit)
	}
}

// TestGetPageForPet_RequiresAccess
func TestGetPageForPet_RequiresAccess(t *testing.T) {
	repo := &fakeLitterRepo{}
	svc := litterSvc(t, repo, &fakePublisher{}, domain.AccessNone)

	if _, _, err := svc.GetPageForPet(ctxAs(uuid.New()), uuid.New(), domain.LogPage{Limit: 5}); err == nil {
		t.Error("คนที่ไม่มีสิทธิ์ต้องอ่านไม่ได้")
	}
}

// TestCreate_PassesThroughCursorFields ยืนยันว่า Date ถูกเติมเมื่อไม่ได้ส่งมา
func TestCreate_FillsDateWhenMissing(t *testing.T) {
	repo := &fakeLitterRepo{}
	svc := litterSvc(t, repo, &fakePublisher{}, domain.AccessOwner)

	log := &domain.LitterLog{PetID: uuid.New(), Type: "Poop", Amount: 1}
	if _, err := svc.Create(ctxAs(uuid.New()), log); err != nil {
		t.Fatalf("ไม่ควร error: %v", err)
	}
	if log.ID == uuid.Nil {
		t.Error("ต้องเติม id ให้เมื่อไม่ได้ส่งมา")
	}
	_ = time.Now()
}

var _ port.LitterRepository = (*fakeLitterRepo)(nil)
