package application

import "context"

// MasterDataService implements port.MasterDataUseCase.
//
// ⚠️ ค่ายังถูก hardcode อยู่ตรงนี้ — Phase 3 จะเปลี่ยนไปอ่านจากตาราง mst_*
// ที่ V3 seed ไว้แล้ว และเปิดให้ backoffice แก้ผ่าน admin CRUD API
// response ของ v1 ต้องคืนค่าเหมือนเดิมทุกตัวอักษร (มี golden test เฝ้าอยู่)
type MasterDataService struct{}

func NewMasterDataService() *MasterDataService { return &MasterDataService{} }

func (s *MasterDataService) GetCatBreeds(_ context.Context) []string {
	return []string{
		"Scottish Fold (หูพับ)", "Scottish Straight (หูตั้ง)", "British Shorthair",
		"Persian", "Maine Coon", "Siamese (วิเชียรมาศ)", "Khao Manee (ขาวมณี)",
		"Sphynx", "Bengal", "Ragdoll", "American Shorthair",
		"Exotic Shorthair", "Munchkin (ขาสั้น)", "Mixed / Other (พันธุ์ผสม/อื่นๆ)",
	}
}

func (s *MasterDataService) GetBloodTypes(_ context.Context) []string {
	return []string{"Unknown", "A", "B", "AB"}
}
