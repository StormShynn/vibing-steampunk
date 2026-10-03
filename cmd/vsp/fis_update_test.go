package main

import "testing"

func TestFISParseVersion(t *testing.T) {
	cases := []struct {
		in   string
		want [3]int
		ok   bool
	}{
		{"fis-2026.10.03", [3]int{2026, 1003, 0}, true},
		{"FIS-2026.10.03.2", [3]int{2026, 1003, 2}, true},
		{"fis-2026.13.01", [3]int{}, false},
		{"fis-dev", [3]int{}, false},
		{"v2.57.0", [3]int{2, 57, 0}, true},
	}
	for _, c := range cases {
		got, ok := parseVersion(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseVersion(%q) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
	a, _ := parseVersion("fis-2026.10.03.1")
	b, _ := parseVersion("fis-2026.09.28")
	if !newerThan(a, b) {
		t.Error("fis-2026.10.03.1 must be newer than fis-2026.09.28")
	}
}
