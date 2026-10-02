package adt

import (
	"strings"
	"testing"
)

func TestBdefExtensionTargets(t *testing.T) {
	cases := []struct {
		src, ext, base, iface string
		wantErr               bool
	}{
		{"managed implementation in class zbp unique;", "", "", "", false},
		{"managed;", "R_X", "", "", true},
		{"extension using interface i_salesordertp\nimplementation in class zbp_x unique;", "r_salesordertp", "R_SALESORDERTP", "I_SALESORDERTP", false},
		{"// header\n/* c */\n  EXTENSION for projection;", "C_SALESORDERMANAGE", "C_SALESORDERMANAGE", "", false},
		{"extension implementation in class zbp unique;", "", "", "", true},
		{"extension;", "bad name!", "", "", true},
	}
	for i, c := range cases {
		b, f, err := bdefExtensionTargets(c.src, c.ext)
		if (err != nil) != c.wantErr || b != c.base || f != c.iface {
			t.Errorf("case %d: got %q %q %v", i, b, f, err)
		}
	}
}

func TestBdefExtensionTemplate(t *testing.T) {
	if bdefExtensionTemplate("", "") != "" {
		t.Fatal("root BDEF must have no template")
	}
	s := bdefExtensionTemplate("R_SALESORDERTP", "I_SALESORDERTP")
	for _, w := range []string{`adtcore:key="base_bdef">R_SALESORDERTP<`, `adtcore:key="interface_bdef">I_SALESORDERTP<`, "<adtcore:adtTemplate>"} {
		if !strings.Contains(s, w) {
			t.Errorf("missing %s in %s", w, s)
		}
	}
}
