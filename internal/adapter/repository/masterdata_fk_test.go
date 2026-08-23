package repository

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestIsForeignKeyViolation
//
// ใช้แยกว่า "ค่าที่ผู้ใช้ส่งมาไม่มีใน master data" ออกจากความผิดพลาดอื่น
// เพื่อตอบ 400 แทน 500 — ถ้าตรวจผิดจะกลืนความผิดพลาดจริงไปเป็น 400
func TestIsForeignKeyViolation(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"FK violation (23503)", &pgconn.PgError{Code: "23503"}, true},
		{"FK ที่ถูก wrap", fmt.Errorf("บันทึกไม่สำเร็จ: %w", &pgconn.PgError{Code: "23503"}), true},
		{"unique violation (23505) ไม่ใช่ FK", &pgconn.PgError{Code: "23505"}, false},
		{"not null (23502) ไม่ใช่ FK", &pgconn.PgError{Code: "23502"}, false},
		{"error ธรรมดา", errors.New("อะไรสักอย่าง"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isForeignKeyViolation(tc.err); got != tc.want {
				t.Errorf("ได้ %v ต้องเป็น %v", got, tc.want)
			}
		})
	}
}
