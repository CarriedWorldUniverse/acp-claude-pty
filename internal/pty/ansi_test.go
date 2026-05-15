package pty

import "testing"

func TestStripANSI(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello world", "hello world"},
		{"sgr-color", "\x1b[31mred\x1b[0m", "red"},
		{"cursor-move", "a\x1b[2Ab", "ab"},
		{"clear-screen", "\x1b[2Jpost", "post"},
		{"osc-title-bel", "\x1b]0;title\x07rest", "rest"},
		{"osc-title-st", "\x1b]0;title\x1b\\rest", "rest"},
		{"charset", "\x1b(Bplain", "plain"},
		{"reset-2byte", "\x1b=on", "on"},
		{"preserves-cr-newline", "line1\r\nline2", "line1\r\nline2"},
		{"multiple-csi", "\x1b[1;34mA\x1b[0m \x1b[1;32mB\x1b[0m", "A B"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(StripANSI([]byte(tc.in)))
			if got != tc.want {
				t.Errorf("StripANSI(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
