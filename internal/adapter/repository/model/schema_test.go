package model

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func parse(t *testing.T, v any) *schema.Schema {
	t.Helper()
	s, err := schema.Parse(v, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	return s
}

// TestTableNames ล็อกชื่อตารางไว้ — เปลี่ยนแล้วข้อมูลเดิมหาย
func TestTableNames(t *testing.T) {
	cases := []struct {
		model any
		table string
	}{
		{&Pet{}, "pets"},
		{&Permission{}, "pet_permissions"},
		{&Caregiver{}, "pet_caregivers"},
		{&Litter{}, "litter_logs"},
		{&Water{}, "water_logs"},
	}
	for _, tc := range cases {
		if got := parse(t, tc.model).Table; got != tc.table {
			t.Errorf("table = %q ต้องการ %q", got, tc.table)
		}
	}
}

// TestJoinTableColumnNames เฝ้า column ของ many2many join table
//
// GORM สร้างชื่อ column จาก "ชื่อ struct" (schema.Name) ไม่ใช่ชื่อตาราง
// การเปลี่ยนชื่อ struct จึงเปลี่ยน column ที่ GORM ไปมองหาโดยไม่มีอะไรเตือน
// ผลคือ permission ที่ caregiver ผูกไว้เดิมจะหายทั้งหมดแบบเงียบๆ
//
// เคยเกิดจริงตอนแยก package model (CaregiverModel → Caregiver)
// จับได้ก่อนขึ้น production ด้วย test ตัวนี้
func TestJoinTableColumnNames(t *testing.T) {
	rel := parse(t, &Caregiver{}).Relationships.Relations["Permissions"]
	if rel == nil {
		t.Fatal("ไม่พบ relation Permissions")
	}
	if rel.JoinTable.Table != "caregiver_permissions" {
		t.Fatalf("join table = %q ต้องการ caregiver_permissions", rel.JoinTable.Table)
	}

	got := map[string]bool{}
	for _, f := range rel.JoinTable.Fields {
		if f.DBName != "" {
			got[f.DBName] = true
		}
	}
	for _, want := range []string{"caregiver_model_id", "permission_model_id"} {
		if !got[want] {
			t.Errorf("ไม่พบ column %q ใน join table (ได้ %v)\n"+
				"ชื่อ column นี้ถูกกำหนดโดย tag joinForeignKey/joinReferences ห้ามลบออก",
				want, keys(got))
		}
	}
}

// TestColumnNames ล็อกชื่อ column ที่ query ดิบใน repository อ้างถึง
func TestColumnNames(t *testing.T) {
	pet := parse(t, &Pet{})
	for _, want := range []string{"id", "owner_id", "owner_username", "deleted_at", "avatar_data"} {
		if pet.LookUpField(want) == nil {
			t.Errorf("pets ต้องมี column %q", want)
		}
	}
	cg := parse(t, &Caregiver{})
	for _, want := range []string{"pet_id", "user_id", "deleted_at"} {
		if cg.LookUpField(want) == nil {
			t.Errorf("pet_caregivers ต้องมี column %q", want)
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
