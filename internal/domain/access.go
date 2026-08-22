package domain

// AccessLevel คือความสัมพันธ์ระหว่าง actor กับสัตว์เลี้ยงหนึ่งตัว
type AccessLevel int

const (
	// AccessNone ไม่มีความเกี่ยวข้องใดๆ — หรือไม่มีสัตว์เลี้ยงตัวนั้นอยู่จริง
	// สองกรณีนี้ต้องแยกไม่ออกจากมุมของ client เพื่อไม่ให้ไล่เดา UUID ได้
	AccessNone AccessLevel = iota
	// AccessCaregiver เป็นผู้ดูแลร่วม สิทธิ์ขึ้นกับ Permissions
	AccessCaregiver
	// AccessOwner เป็นเจ้าของ ทำได้ทุกอย่างกับสัตว์เลี้ยงตัวนั้น
	AccessOwner
)

// Capability คือสิทธิ์ระดับ global ที่มาจาก role (ข้ามความเป็นเจ้าของได้)
const (
	CapPetReadAny         = "pet:read:any"
	CapPetWriteAny        = "pet:write:any"
	CapPetDeleteAny       = "pet:delete:any"
	CapCaregiverManageAny = "caregiver:manage:any"
	CapLogReadAny         = "log:read:any"
	CapLogWriteAny        = "log:write:any"
	CapMasterDataWrite    = "masterdata:write"
)

// Permission คือสิทธิ์ระดับ resource ที่เจ้าของมอบให้ caregiver
const (
	PermEditProfile   = "EDIT_PROFILE"
	PermManageMedical = "MANAGE_MEDICAL"
	PermManageWeight  = "MANAGE_WEIGHT"
	PermManageTasks   = "MANAGE_TASKS"
	PermManageLitter  = "MANAGE_LITTER"
	PermManageWater   = "MANAGE_WATER"
)

// PetAccess คือผลการตรวจสิทธิ์ของ actor หนึ่งคนต่อสัตว์เลี้ยงหนึ่งตัว
type PetAccess struct {
	Level       AccessLevel
	Permissions []string
}

// Has บอกว่าทำสิ่งที่ต้องการ permission ตัวนี้ได้ไหม
// เจ้าของทำได้ทุกอย่างเสมอ ไม่ต้องมีแถวใน caregiver_permissions
func (a PetAccess) Has(permission string) bool {
	if a.Level == AccessOwner {
		return true
	}
	if a.Level != AccessCaregiver {
		return false
	}
	for _, p := range a.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}
