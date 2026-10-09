package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
)

// fakeAccessByUserRepo คืนค่า FindAccess ต่างกันตาม userID ที่ถาม — ต่างจาก
// fakePetRepo เดิมใน fakes_test.go ที่คืนค่าเดียวตายตัวไม่ว่าถามใคร ใช้ไม่ได้
// กับเทสนี้ที่ต้องแยกผลของสองคนออกจากกัน
type fakeAccessByUserRepo struct {
	fakePetRepo
	byUser map[uuid.UUID]domain.PetAccess
	err    error
}

func (f *fakeAccessByUserRepo) FindAccess(_ context.Context, _ uuid.UUID, userID uuid.UUID) (domain.PetAccess, error) {
	if f.err != nil {
		return domain.PetAccess{}, f.err
	}
	return f.byUser[userID], nil
}

func TestPetLinkService_IsLinked_BothRelated(t *testing.T) {
	owner, caregiver := uuid.New(), uuid.New()
	repo := &fakeAccessByUserRepo{byUser: map[uuid.UUID]domain.PetAccess{
		owner:     {Level: domain.AccessOwner},
		caregiver: {Level: domain.AccessCaregiver},
	}}
	svc := NewPetLinkService(repo)

	linked, err := svc.IsLinked(context.Background(), uuid.New(), owner, caregiver)
	if err != nil {
		t.Fatal(err)
	}
	if !linked {
		t.Fatal("เจ้าของกับผู้ดูแลของสัตว์เลี้ยงตัวเดียวกัน ต้องได้ linked=true")
	}
}

func TestPetLinkService_IsLinked_OneSideUnrelated(t *testing.T) {
	owner, stranger := uuid.New(), uuid.New()
	repo := &fakeAccessByUserRepo{byUser: map[uuid.UUID]domain.PetAccess{
		owner: {Level: domain.AccessOwner},
		// stranger ไม่มีใน map เลย — FindAccess คืน zero value = AccessNone
	}}
	svc := NewPetLinkService(repo)

	linked, err := svc.IsLinked(context.Background(), uuid.New(), owner, stranger)
	if err != nil {
		t.Fatal(err)
	}
	if linked {
		t.Fatal("stranger ไม่เกี่ยวข้องกับสัตว์เลี้ยงตัวนี้เลย ต้องได้ linked=false")
	}
}

func TestPetLinkService_IsLinked_PropagatesRepoError(t *testing.T) {
	repo := &fakeAccessByUserRepo{err: errors.New("db ล่ม")}
	svc := NewPetLinkService(repo)

	_, err := svc.IsLinked(context.Background(), uuid.New(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("ต้องส่ง error ของ repository กลับไปให้ caller แปลงเอง ไม่ใช่กลืนเงียบๆ")
	}
}
