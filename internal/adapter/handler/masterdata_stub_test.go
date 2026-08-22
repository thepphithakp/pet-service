package handler

import "context"

// masterDataStub คืนค่าคงที่เพื่อยืนยัน response shape ของ v1
type masterDataStub struct{}

func (masterDataStub) GetCatBreeds(context.Context) []string {
	return []string{"Scottish Fold (หูพับ)", "Persian"}
}
func (masterDataStub) GetBloodTypes(context.Context) []string {
	return []string{"Unknown", "A"}
}
