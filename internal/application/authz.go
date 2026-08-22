package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
)

// Requirement บอกว่าการกระทำหนึ่งต้องการอะไร
//
// ผ่านได้สองทาง:
//  1. Capabilities — สิทธิ์ระดับ global จาก role (admin ข้ามความเป็นเจ้าของได้)
//  2. MinLevel + Permission — ความสัมพันธ์กับสัตว์เลี้ยงตัวนั้นโดยตรง
type Requirement struct {
	// Capabilities ผ่านถ้ามีตัวใดตัวหนึ่ง (OR)
	Capabilities []string
	// MinLevel ระดับความสัมพันธ์ขั้นต่ำ
	MinLevel domain.AccessLevel
	// Permission สิทธิ์ที่ caregiver ต้องมี ("" = ไม่ต้องมี) เจ้าของผ่านเสมอ
	Permission string
}

// Authorizer ตรวจสิทธิ์ก่อนทุกการกระทำที่ผูกกับสัตว์เลี้ยงตัวใดตัวหนึ่ง
//
// อยู่ที่ชั้น application ไม่ใช่ handler โดยตั้งใจ —
// ถ้าเช็คที่ handler แล้ววันหนึ่งมี caller ใหม่ (gRPC, cron, message consumer)
// เรียก use case ตรงๆ การตรวจสิทธิ์จะถูกข้ามไปเงียบๆ
type Authorizer struct {
	pets port.PetRepository
	caps port.CapabilityRepository
}

func NewAuthorizer(pets port.PetRepository, caps port.CapabilityRepository) *Authorizer {
	return &Authorizer{pets: pets, caps: caps}
}

// Authorize คืน nil เมื่อผ่าน
//
// เมื่อไม่ผ่านจะคืน:
//   - ErrUnauthenticated ถ้าไม่มี actor ใน context
//   - ErrForbidden       ถ้า actor เห็น resource นี้ได้อยู่แล้ว (เป็น caregiver) แต่สิทธิ์ไม่พอ
//   - ErrPetNotFound     ถ้า actor ไม่เกี่ยวข้องกับ resource นี้เลย หรือ resource ไม่มีอยู่จริง
//
// ⚠️ ความต่างระหว่างสองอันหลังสำคัญมาก: การตอบ 403 กับสิ่งที่ actor ไม่ควรรู้ว่ามีอยู่
//
//	เท่ากับยืนยันว่า UUID นั้นมีจริง ทำให้ไล่เดาข้อมูลในระบบได้
func (a *Authorizer) Authorize(ctx context.Context, petID uuid.UUID, req Requirement) error {
	actor, ok := domain.ActorFromContext(ctx)
	if !ok {
		return domain.ErrUnauthenticated
	}

	// ทางที่ 1 — capability ระดับ global
	if len(req.Capabilities) > 0 {
		allowed, err := a.caps.HasAny(ctx, actor.Roles, req.Capabilities...)
		if err != nil {
			return fmt.Errorf("ตรวจ capability ไม่สำเร็จ: %w", err)
		}
		if allowed {
			return nil
		}
	}

	// ทางที่ 2 — ความสัมพันธ์กับสัตว์เลี้ยงตัวนั้น
	access, err := a.pets.FindAccess(ctx, petID, actor.UserID)
	if err != nil {
		return fmt.Errorf("ตรวจสิทธิ์ต่อสัตว์เลี้ยงไม่สำเร็จ: %w", err)
	}

	if access.Level < req.MinLevel || access.Level == domain.AccessNone {
		if access.Level == domain.AccessNone {
			return domain.ErrPetNotFound
		}
		return domain.ErrForbidden
	}
	if req.Permission != "" && !access.Has(req.Permission) {
		return domain.ErrForbidden
	}
	return nil
}

// AuthorizeGlobal ตรวจเฉพาะ capability สำหรับ endpoint ที่ไม่ผูกกับสัตว์เลี้ยงตัวใดตัวหนึ่ง
// เช่น GET /admin/pets หรือ admin master data API
func (a *Authorizer) AuthorizeGlobal(ctx context.Context, capabilities ...string) error {
	actor, ok := domain.ActorFromContext(ctx)
	if !ok {
		return domain.ErrUnauthenticated
	}
	allowed, err := a.caps.HasAny(ctx, actor.Roles, capabilities...)
	if err != nil {
		return fmt.Errorf("ตรวจ capability ไม่สำเร็จ: %w", err)
	}
	if !allowed {
		return domain.ErrForbidden
	}
	return nil
}

// --- Requirement ที่ใช้ซ้ำ รวมไว้ที่เดียวเพื่อให้เห็นภาพรวมสิทธิ์ทั้งระบบ ---

var (
	ReqPetRead = Requirement{
		Capabilities: []string{domain.CapPetReadAny},
		MinLevel:     domain.AccessCaregiver,
	}
	ReqPetUpdate = Requirement{
		Capabilities: []string{domain.CapPetWriteAny},
		MinLevel:     domain.AccessCaregiver,
		Permission:   domain.PermEditProfile,
	}
	ReqPetDelete = Requirement{
		Capabilities: []string{domain.CapPetDeleteAny},
		MinLevel:     domain.AccessOwner,
	}
	ReqCaregiverManage = Requirement{
		Capabilities: []string{domain.CapCaregiverManageAny},
		MinLevel:     domain.AccessOwner,
	}
	ReqLogRead = Requirement{
		Capabilities: []string{domain.CapLogReadAny, domain.CapPetReadAny},
		MinLevel:     domain.AccessCaregiver,
	}
	ReqLitterWrite = Requirement{
		Capabilities: []string{domain.CapLogWriteAny},
		MinLevel:     domain.AccessCaregiver,
		Permission:   domain.PermManageLitter,
	}
	ReqWaterWrite = Requirement{
		Capabilities: []string{domain.CapLogWriteAny},
		MinLevel:     domain.AccessCaregiver,
		Permission:   domain.PermManageWater,
	}
)
