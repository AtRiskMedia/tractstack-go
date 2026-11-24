package services

import (
	"testing"
)

func TestReservedAdminUsername(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"admin", true},
		{"Admin", true},
		{"ADMIN", true},
		{" admin ", true},
		{"administrator", false},
		{"editor", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := reservedAdminUsername(tc.in); got != tc.want {
			t.Errorf("reservedAdminUsername(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
}

func TestMainPasswordTV_ChangesWithHash(t *testing.T) {
	a := mainPasswordTV("hash-a")
	b := mainPasswordTV("hash-b")
	if a == b {
		t.Fatal("tv should change when hash changes")
	}
	if len(a) != 16 {
		t.Fatalf("expected 16 hex chars, got %d", len(a))
	}
}
