package domain

import "time"

// MasterDataType คือชนิดของ master data ที่เปิดให้จัดการผ่าน backoffice
//
// ค่าเหล่านี้เป็นส่วนหนึ่งของ URL (/admin/master-data/{type})
// และถูกใช้เลือกตารางผ่าน allowlist ที่ชั้น repository
// จึงไม่มีทางที่ค่าจาก client จะไหลลงไปเป็นชื่อตารางใน SQL โดยตรง
type MasterDataType string

const (
	MasterSpecies     MasterDataType = "species"
	MasterCatBreeds   MasterDataType = "cat-breeds"
	MasterBloodTypes  MasterDataType = "blood-types"
	MasterLitterTypes MasterDataType = "litter-types"
	MasterGenders     MasterDataType = "genders"
)

// AllMasterDataTypes คือชนิดที่เปิด CRUD ให้ backoffice ทั้งหมด
//
// 🚫 pet_permissions และ role_capabilities ไม่อยู่ในนี้โดยตั้งใจ
//
//	สองตัวนั้นผูกกับโค้ดที่บังคับใช้จริง เพิ่มผ่าน UI แล้วจะไม่มีผลอะไร
//	จึงจัดการด้วย repeatable migration (db/codeowned) แทน
var AllMasterDataTypes = []MasterDataType{
	MasterSpecies, MasterCatBreeds, MasterBloodTypes, MasterLitterTypes, MasterGenders,
}

func (t MasterDataType) Valid() bool {
	for _, v := range AllMasterDataTypes {
		if v == t {
			return true
		}
	}
	return false
}

// MasterDataItem คือรายการหนึ่งใน master data
type MasterDataItem struct {
	Code   string  `json:"code"`
	NameEn string  `json:"nameEn"`
	NameTh *string `json:"nameTh,omitempty"`

	// LegacyLabel คือสตริงที่ API v1 เคยคืนแบบตรงตัวอักษร
	// เช่น "Scottish Fold (หูพับ)" — มีเฉพาะ cat-breeds และ blood-types
	//
	// เก็บไว้เพื่อให้ย้าย master data เข้าฐานข้อมูลได้โดยที่ client เดิมไม่พัง
	LegacyLabel *string `json:"legacyLabel,omitempty"`

	// SpeciesCode มีเฉพาะ cat-breeds
	SpeciesCode *string `json:"speciesCode,omitempty"`

	SortOrder int  `json:"sortOrder"`
	IsActive  bool `json:"isActive"`

	// Version สำหรับ optimistic locking — กัน admin สองคนแก้ชนกัน
	Version int `json:"version"`

	CreatedAt time.Time `json:"createdAt"`
	CreatedBy *string   `json:"createdBy,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
	UpdatedBy *string   `json:"updatedBy,omitempty"`
}

// DisplayLabel คืนค่าที่ API v1 ต้องคืน
//
// ใช้ legacy_label ถ้ามี ไม่งั้นใช้ name_en
// รายการที่ admin เพิ่มใหม่ผ่าน backoffice จะไม่มี legacy_label
// จึงคืน name_en ซึ่งเป็นพฤติกรรมที่ถูกต้องสำหรับค่าที่ไม่เคยมีใน v1 มาก่อน
func (m MasterDataItem) DisplayLabel() string {
	if m.LegacyLabel != nil && *m.LegacyLabel != "" {
		return *m.LegacyLabel
	}
	return m.NameEn
}
