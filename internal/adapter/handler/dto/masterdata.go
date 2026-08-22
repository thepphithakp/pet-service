package dto

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/vertex/pet-service/internal/domain"
)

// codePattern บังคับให้ code เป็นตัวพิมพ์ใหญ่ ตัวเลข และ underscore
//
// 🔸 ยกเว้น litter-types และ genders ที่ code ต้องตรงกับค่าที่เก็บอยู่จริง
//
//	ในข้อมูล (เช่น "Poop" ไม่ใช่ "POOP") เพราะค่านั้นถูกส่งกลับให้ client
//	ทาง API ตรงๆ — ดู db/migration/V3__seed_masterdata_initial.sql
var codePattern = regexp.MustCompile(`^[A-Z0-9_]{1,50}$`)

// looseCodePattern สำหรับชนิดที่ code ผูกกับค่าที่มีอยู่แล้วในข้อมูลจริง
var looseCodePattern = regexp.MustCompile(`^[\p{L}0-9 _-]{1,50}$`)

func codeRuleFor(t domain.MasterDataType) *regexp.Regexp {
	switch t {
	case domain.MasterLitterTypes, domain.MasterGenders:
		return looseCodePattern
	default:
		return codePattern
	}
}

type CreateMasterDataRequest struct {
	Code        string  `json:"code"`
	NameEn      string  `json:"nameEn"`
	NameTh      *string `json:"nameTh"`
	SpeciesCode *string `json:"speciesCode"`
	SortOrder   int     `json:"sortOrder"`
	IsActive    *bool   `json:"isActive"`
}

func (r CreateMasterDataRequest) Validate(t domain.MasterDataType) error {
	var errs []string

	code := strings.TrimSpace(r.Code)
	if code == "" {
		errs = append(errs, "code: ต้องระบุ")
	} else if !codeRuleFor(t).MatchString(code) {
		errs = append(errs, fmt.Sprintf("code: รูปแบบไม่ถูกต้อง (ต้องตรงกับ %s)", codeRuleFor(t)))
	}

	if strings.TrimSpace(r.NameEn) == "" {
		errs = append(errs, "nameEn: ต้องระบุ")
	}
	if len([]rune(r.NameEn)) > 200 {
		errs = append(errs, "nameEn: ยาวเกิน 200 ตัวอักษร")
	}
	if r.NameTh != nil && len([]rune(*r.NameTh)) > 200 {
		errs = append(errs, "nameTh: ยาวเกิน 200 ตัวอักษร")
	}
	if r.SortOrder < 0 || r.SortOrder > 9999 {
		errs = append(errs, "sortOrder: ต้องอยู่ระหว่าง 0 ถึง 9999")
	}

	// cat-breeds ต้องผูกกับสายพันธุ์สัตว์เสมอ
	if t == domain.MasterCatBreeds && (r.SpeciesCode == nil || strings.TrimSpace(*r.SpeciesCode) == "") {
		errs = append(errs, "speciesCode: ต้องระบุสำหรับ cat-breeds")
	}

	if len(errs) > 0 {
		return fmt.Errorf("%w: %s", domain.ErrValidation, strings.Join(errs, ", "))
	}
	return nil
}

func (r CreateMasterDataRequest) ToDomain() domain.MasterDataItem {
	active := true
	if r.IsActive != nil {
		active = *r.IsActive
	}
	return domain.MasterDataItem{
		Code:        strings.TrimSpace(r.Code),
		NameEn:      strings.TrimSpace(r.NameEn),
		NameTh:      r.NameTh,
		SpeciesCode: r.SpeciesCode,
		SortOrder:   r.SortOrder,
		IsActive:    active,
		// 🚫 จงใจไม่รับ legacyLabel จาก client
		//    ค่านั้นมีไว้เพื่อรักษาความเข้ากันได้ของ API v1 กับข้อมูลที่ seed มา
		//    รายการที่เพิ่มใหม่ไม่เคยมีใน v1 จึงไม่ต้องมี และไม่ควรให้ตั้งเองได้
	}
}

// UpdateMasterDataRequest — code ไม่อยู่ในนี้เพราะมาจาก path และเปลี่ยนไม่ได้
type UpdateMasterDataRequest struct {
	NameEn      string  `json:"nameEn"`
	NameTh      *string `json:"nameTh"`
	SpeciesCode *string `json:"speciesCode"`
	SortOrder   int     `json:"sortOrder"`
	IsActive    *bool   `json:"isActive"`
	// Version ต้องส่งมาเสมอ เพื่อกัน admin สองคนแก้ทับกัน
	Version int `json:"version"`
}

func (r UpdateMasterDataRequest) Validate(t domain.MasterDataType) error {
	var errs []string
	if strings.TrimSpace(r.NameEn) == "" {
		errs = append(errs, "nameEn: ต้องระบุ")
	}
	if len([]rune(r.NameEn)) > 200 {
		errs = append(errs, "nameEn: ยาวเกิน 200 ตัวอักษร")
	}
	if r.NameTh != nil && len([]rune(*r.NameTh)) > 200 {
		errs = append(errs, "nameTh: ยาวเกิน 200 ตัวอักษร")
	}
	if r.SortOrder < 0 || r.SortOrder > 9999 {
		errs = append(errs, "sortOrder: ต้องอยู่ระหว่าง 0 ถึง 9999")
	}
	if r.Version <= 0 {
		errs = append(errs, "version: ต้องส่งค่า version ที่ได้จากการอ่านครั้งล่าสุด")
	}
	if t == domain.MasterCatBreeds && (r.SpeciesCode == nil || strings.TrimSpace(*r.SpeciesCode) == "") {
		errs = append(errs, "speciesCode: ต้องระบุสำหรับ cat-breeds")
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %s", domain.ErrValidation, strings.Join(errs, ", "))
	}
	return nil
}

func (r UpdateMasterDataRequest) ToDomain() domain.MasterDataItem {
	active := true
	if r.IsActive != nil {
		active = *r.IsActive
	}
	return domain.MasterDataItem{
		NameEn:      strings.TrimSpace(r.NameEn),
		NameTh:      r.NameTh,
		SpeciesCode: r.SpeciesCode,
		SortOrder:   r.SortOrder,
		IsActive:    active,
		Version:     r.Version,
	}
}

// MasterDataResponse คือรูปแบบของ API v2
type MasterDataResponse struct {
	Code        string    `json:"code"`
	NameEn      string    `json:"nameEn"`
	NameTh      *string   `json:"nameTh,omitempty"`
	Label       string    `json:"label"`
	SpeciesCode *string   `json:"speciesCode,omitempty"`
	SortOrder   int       `json:"sortOrder"`
	IsActive    bool      `json:"isActive"`
	Version     int       `json:"version"`
	UpdatedAt   time.Time `json:"updatedAt"`
	UpdatedBy   *string   `json:"updatedBy,omitempty"`
}

func ToMasterDataResponse(m domain.MasterDataItem) MasterDataResponse {
	return MasterDataResponse{
		Code: m.Code, NameEn: m.NameEn, NameTh: m.NameTh,
		// Label คือค่าเดียวกับที่ v1 คืน ทำให้ client ย้ายมา v2 ได้ทีละขั้น
		Label:       m.DisplayLabel(),
		SpeciesCode: m.SpeciesCode,
		SortOrder:   m.SortOrder, IsActive: m.IsActive, Version: m.Version,
		UpdatedAt: m.UpdatedAt, UpdatedBy: m.UpdatedBy,
	}
}

func ToMasterDataResponses(items []domain.MasterDataItem) []MasterDataResponse {
	out := make([]MasterDataResponse, len(items))
	for i, m := range items {
		out[i] = ToMasterDataResponse(m)
	}
	return out
}
