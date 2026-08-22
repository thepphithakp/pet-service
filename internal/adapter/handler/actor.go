package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/vertex/pet-service/internal/domain"
)

// setLogActor เซ็ตผู้บันทึกจาก actor ใน context เท่านั้น
//
// ห้ามรับ createdBy จาก request body — เดิม handler bind domain object ตรงๆ
// ทำให้ client ส่ง createdBy มาเองได้ (S-3)
func setLogActor(c *fiber.Ctx, createdBy, createdByUsername **string) {
	actor, ok := domain.ActorFromContext(c.UserContext())
	if !ok {
		return
	}
	uid := actor.UserID.String()
	*createdBy = &uid
	if actor.Username != "" {
		name := actor.Username
		*createdByUsername = &name
	}
}
