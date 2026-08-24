package middleware

import "testing"

func TestNormalizeEndpoint(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/api/v1/pets/86715873-9C34-4448-ABE1-EA9184E71553", "/api/v1/pets/:id"},
		{"/api/v1/pets/00000000-0000-0000-0000-000000000000/water-logs", "/api/v1/pets/:id/water-logs"},
		{"/api/v1/pets/:id/water-logs/AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE", "/api/v1/pets/:id/water-logs/:id"},
		{"/api/v1/master-data/species", "/api/v1/master-data/species"}, // ไม่ใช่ UUID ไม่ต้องแตะ
		{"/api/v1/pets", "/api/v1/pets"},
		{"/", "/"},
	}
	for _, tc := range cases {
		if got := normalizeEndpoint(tc.in); got != tc.want {
			t.Errorf("normalizeEndpoint(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
