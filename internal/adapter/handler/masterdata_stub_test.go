package handler

import (
	"context"

	"github.com/vertex/pet-service/internal/domain"
)

// masterDataStub คืนค่าคงที่เพื่อยืนยัน response shape ของ v1
type masterDataStub struct{}

func (masterDataStub) GetCatBreeds(context.Context) []string {
	return []string{"Scottish Fold (หูพับ)", "Persian"}
}
func (masterDataStub) GetBloodTypes(context.Context) []string {
	return []string{"Unknown", "A"}
}
func (masterDataStub) List(context.Context, domain.MasterDataType) ([]domain.MasterDataItem, error) {
	return []domain.MasterDataItem{{Code: "PERSIAN", NameEn: "Persian", SortOrder: 40, IsActive: true, Version: 1}}, nil
}
func (masterDataStub) Permissions(context.Context) ([]domain.PetPermission, error) {
	return []domain.PetPermission{{ID: "EDIT_PROFILE", Name: "Edit Profile", IsActive: true}}, nil
}
func (masterDataStub) IsValid(context.Context, domain.MasterDataType, string) bool { return true }

func (masterDataStub) ListAll(context.Context, domain.MasterDataType) ([]domain.MasterDataItem, error) {
	return nil, nil
}
func (masterDataStub) Create(context.Context, domain.MasterDataType, domain.MasterDataItem) (*domain.MasterDataItem, error) {
	return nil, nil
}
func (masterDataStub) Update(context.Context, domain.MasterDataType, domain.MasterDataItem) (*domain.MasterDataItem, error) {
	return nil, nil
}
func (masterDataStub) Deactivate(context.Context, domain.MasterDataType, string) (int64, error) {
	return 0, nil
}
func (masterDataStub) UsageCount(context.Context, domain.MasterDataType, string) (int64, error) {
	return 0, nil
}
