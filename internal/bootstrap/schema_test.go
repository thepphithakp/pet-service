package bootstrap

import "testing"

func TestMajorVersion(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"8", 8, false},
		{"8.1", 8, false},
		{"10.2.3", 10, false},
		{" 7 ", 7, false},
		{"", 0, true},
		{"abc", 0, true},
	}
	for _, tc := range cases {
		got, err := majorVersion(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("majorVersion(%q) ต้องคืน error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("majorVersion(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("majorVersion(%q) = %d ต้องการ %d", tc.in, got, tc.want)
		}
	}
}
