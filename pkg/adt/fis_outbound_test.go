package adt

import (
	"strings"
	"testing"
)

func TestBuildOutboundServiceContent(t *testing.T) {
	s, err := buildOutboundServiceContent("ZOS_INT_BANK_COTA", OutboundServiceOptions{CommunicationTarget: "zct_int_bank"})
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"<sco3:id>ZOS_INT_BANK_COTA</sco3:id>", "<sco3:type>COTA</sco3:type>", "<sco3:cotaId>ZCT_INT_BANK</sco3:cotaId>", "<sco3:urlPath/>", "<sco3:cotaComType>H</sco3:cotaComType>"} {
		if !strings.Contains(s, w) {
			t.Errorf("missing %s", w)
		}
	}
	if _, err := buildOutboundServiceContent("ZOS_INT_BANK", OutboundServiceOptions{CommunicationTarget: "ZCT_X"}); err == nil {
		t.Error("COTA type without _COTA suffix must be refused")
	}
	if _, err := buildOutboundServiceContent("ZOS_X_COTA", OutboundServiceOptions{}); err == nil {
		t.Error("COTA type without target must be refused")
	}
	r, err := buildOutboundServiceContent("ZOS_X_REST", OutboundServiceOptions{Type: "rest", URLPath: "/api"})
	if err != nil || !strings.Contains(r, "<sco3:urlPath>/api</sco3:urlPath>") || !strings.Contains(r, "<sco3:abapLanguageVersion>5</sco3:abapLanguageVersion>") {
		t.Errorf("REST: %v %s", err, r)
	}
	if _, err := buildOutboundServiceContent("ZOS_X", OutboundServiceOptions{Type: "RFC"}); err == nil {
		t.Error("unknown type must be refused")
	}
}

func TestBuildOutboundServicesXML(t *testing.T) {
	if buildOutboundServicesXML(nil) != "    <sco1:outboundServices/>" {
		t.Error("empty list")
	}
	s := buildOutboundServicesXML([]string{" zos_a_cota", "", "ZOS_B_REST"})
	for _, w := range []string{"<sco1:outboundID>0001</sco1:outboundID><sco1:obsID>ZOS_A_COTA</sco1:obsID><sco1:obsType>COTA</sco1:obsType>", "<sco1:outboundID>0002</sco1:outboundID><sco1:obsID>ZOS_B_REST</sco1:obsID><sco1:obsType>REST</sco1:obsType>"} {
		if !strings.Contains(s, w) {
			t.Errorf("missing %s in %s", w, s)
		}
	}
}
