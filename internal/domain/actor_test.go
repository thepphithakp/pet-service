package domain

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestActor_HasRole(t *testing.T) {
	actor := Actor{Roles: []string{RoleUser, RolePetAdmin}}

	cases := []struct {
		role string
		want bool
	}{
		{RoleUser, true},
		{RolePetAdmin, true},
		{RoleSuperAdmin, false},
		{"", false},
		// ตัวพิมพ์ต้องตรง — role เป็นค่าคงที่ ไม่ใช่ข้อความอิสระ
		{"user", false},
	}
	for _, tc := range cases {
		if got := actor.HasRole(tc.role); got != tc.want {
			t.Errorf("HasRole(%q) = %v ต้องเป็น %v", tc.role, got, tc.want)
		}
	}
}

func TestActor_HasRoleOnEmptyActor(t *testing.T) {
	var actor Actor
	if actor.HasRole(RoleSuperAdmin) {
		t.Error("actor ที่ไม่มี role ต้องไม่ผ่านการตรวจใดๆ")
	}
}

func TestActorFromContext(t *testing.T) {
	t.Run("ไม่มี actor", func(t *testing.T) {
		if _, ok := ActorFromContext(context.Background()); ok {
			t.Error("context เปล่าต้องไม่มี actor")
		}
	})

	t.Run("มี actor", func(t *testing.T) {
		id := uuid.New()
		ctx := WithActor(context.Background(), Actor{UserID: id, Roles: []string{RoleUser}})
		got, ok := ActorFromContext(ctx)
		if !ok {
			t.Fatal("ต้องอ่าน actor กลับมาได้")
		}
		if got.UserID != id {
			t.Errorf("UserID = %v ต้องเป็น %v", got.UserID, id)
		}
	})
}
