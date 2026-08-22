package domain

import (
	"context"

	"github.com/google/uuid"
)

// Role คือ role ระดับ global ที่ auth-service ใส่มาใน JWT claim "roles"
const (
	RoleSuperAdmin = "SUPER_ADMIN"
	RolePetAdmin   = "PET_ADMIN"
	RoleUser       = "USER"
)

// Actor คือผู้ที่กำลังเรียก API
//
// แยกจาก "เจ้าของข้อมูล" ให้ชัด — เดิมโค้ดเอา OwnerUsername ไปใส่ในช่อง ActorID
// ของ event log ทำให้ audit trail บอกไม่ได้ว่าใครเป็นคนกระทำจริง (C-2)
type Actor struct {
	UserID   uuid.UUID
	Username string
	Email    string
	Roles    []string
}

func (a Actor) HasRole(role string) bool {
	for _, r := range a.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// IsZero บอกว่า actor ยังไม่ถูกเซ็ต (ไม่ได้ผ่าน auth middleware)
func (a Actor) IsZero() bool { return a.UserID == uuid.Nil }

type actorCtxKey struct{}

// WithActor ผูก actor เข้ากับ context
//
// ใช้ context ไม่ใช่ fiber.Locals อย่างเดียว เพื่อให้ชั้น application เข้าถึงได้
// โดยไม่ต้องรู้จัก HTTP — และเพื่อให้ caller อื่นในอนาคต (gRPC, cron) ใช้ทางเดียวกัน
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorCtxKey{}, a)
}

// ActorFromContext ดึง actor ออกจาก context
func ActorFromContext(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorCtxKey{}).(Actor)
	return a, ok && !a.IsZero()
}
