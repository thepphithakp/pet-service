// Package dto เป็นสัญญาระหว่าง HTTP กับภายใน
//
// handler ต้อง bind DTO เท่านั้น ห้าม bind domain object ตรงๆ
// เพราะ domain มีฟิลด์ที่ client ไม่ควรกำหนดเอง เช่น id, ownerId, createdBy,
// updatedBy, caregivers — การ bind ตรงๆ เปิดช่องให้ mass assignment (S-3)
package dto

import (
	"fmt"
	"strings"
	"time"

	"github.com/vertex/pet-service/internal/domain"
)

const (
	maxNameLen    = 100
	maxTextLen    = 2000
	maxAvatarSize = 2 << 20 // 2MB
	maxWeightKg   = 200
)

// CreatePetRequest — สังเกตว่าไม่มี id, ownerId, createdBy, updatedBy, caregivers
type CreatePetRequest struct {
	Name             string    `json:"name"`
	Species          string    `json:"species"`
	Breed            string    `json:"breed"`
	ColorCode        string    `json:"colorCode"`
	BirthDate        time.Time `json:"birthDate"`
	Gender           string    `json:"gender"`
	AvatarData       []byte    `json:"avatarData,omitempty"`
	CurrentWeight    *float64  `json:"currentWeight"`
	MicrochipId      *string   `json:"microchipId"`
	IsSpayedNeutered bool      `json:"isSpayedNeutered"`
	BloodType        *string   `json:"bloodType"`
	Allergies        *string   `json:"allergies"`
	Personality      *string   `json:"personality"`
}

func (r CreatePetRequest) Validate() error {
	if err := validatePetFields(r.Name, true, r.BirthDate, r.AvatarData, r.CurrentWeight,
		r.MicrochipId, r.Allergies, r.Personality, r.BloodType); err != nil {
		return err
	}
	return validateStrings(map[string]string{
		"species":   r.Species,
		"breed":     r.Breed,
		"colorCode": r.ColorCode,
		"gender":    r.Gender,
	}, maxNameLen)
}

func (r CreatePetRequest) ToDomain() domain.Pet {
	return domain.Pet{
		Name: strings.TrimSpace(r.Name), Species: r.Species, Breed: r.Breed,
		ColorCode: r.ColorCode, BirthDate: r.BirthDate, Gender: r.Gender,
		AvatarData: r.AvatarData, CurrentWeight: r.CurrentWeight, MicrochipId: r.MicrochipId,
		IsSpayedNeutered: r.IsSpayedNeutered, BloodType: r.BloodType,
		Allergies: r.Allergies, Personality: r.Personality,
	}
}

// UpdatePetRequest คงพฤติกรรมเดิมของ PUT ไว้ทุกอย่าง
//
// ค่าว่าง = "ไม่แก้" ซึ่งแปลว่าล้างค่าเป็น null ไม่ได้ (C-3)
// Phase 4.4 จะเพิ่ม PATCH ที่แยก "ไม่ส่ง" ออกจาก "ส่ง null" ได้
// โดยที่ PUT ตัวนี้ยังทำงานเหมือนเดิมเพื่อไม่ให้ client ที่ใช้อยู่พัง
type UpdatePetRequest = CreatePetRequest

// validatePetFields ตรวจเฉพาะโครงสร้าง
//
// ⚠️ ยังไม่ตรวจ species / breed / gender / bloodType กับ master data ที่นี่
// เพราะค่าเหล่านั้นแก้ผ่าน backoffice ได้ การ hardcode enum ไว้ในโค้ดจะทำให้
// admin เพิ่มค่าใหม่แล้วใช้ไม่ได้ — Phase 3 จะเช็คกับ master data ที่ cache ไว้แทน
func validatePetFields(
	name string, nameRequired bool, birthDate time.Time, avatar []byte,
	weight *float64, texts ...*string,
) error {
	var errs []string

	name = strings.TrimSpace(name)
	if nameRequired && name == "" {
		errs = append(errs, "name: ต้องระบุ")
	}
	if len([]rune(name)) > maxNameLen {
		errs = append(errs, fmt.Sprintf("name: ยาวเกิน %d ตัวอักษร", maxNameLen))
	}
	if !birthDate.IsZero() && birthDate.After(time.Now()) {
		errs = append(errs, "birthDate: ต้องไม่อยู่ในอนาคต")
	}
	if len(avatar) > maxAvatarSize {
		errs = append(errs, fmt.Sprintf("avatarData: ใหญ่เกิน %d ไบต์", maxAvatarSize))
	}
	if weight != nil && (*weight <= 0 || *weight > maxWeightKg) {
		errs = append(errs, fmt.Sprintf("currentWeight: ต้องอยู่ระหว่าง 0 ถึง %d", maxWeightKg))
	}
	for _, t := range texts {
		if t != nil && len([]rune(*t)) > maxTextLen {
			errs = append(errs, fmt.Sprintf("มีฟิลด์ข้อความยาวเกิน %d ตัวอักษร", maxTextLen))
			break
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%w: %s", domain.ErrValidation, strings.Join(errs, ", "))
	}
	return nil
}

// validateStrings ตรวจความยาวของฟิลด์ string ธรรมดา
func validateStrings(fields map[string]string, maxLen int) error {
	var errs []string
	for name, v := range fields {
		if len([]rune(v)) > maxLen {
			errs = append(errs, fmt.Sprintf("%s: ยาวเกิน %d ตัวอักษร", name, maxLen))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %s", domain.ErrValidation, strings.Join(errs, ", "))
	}
	return nil
}
