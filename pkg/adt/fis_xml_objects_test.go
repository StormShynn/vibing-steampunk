package adt

import (
	"context"
	"encoding/xml"
	"strings"
	"testing"
)

func wellFormed(t *testing.T, s string) {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader("<r xmlns:auth='a' xmlns:suso='s' xmlns:sco1='c' xmlns:enho='e' xmlns:adtcore='x'>" + s + "</r>"))
	for {
		if _, err := d.Token(); err != nil {
			if err.Error() == "EOF" {
				return
			}
			t.Fatalf("not well-formed: %v\n%s", err, s)
		}
	}
}

func TestAuthObjectContent(t *testing.T) {
	s, err := buildAuthObjectContent(AuthObjectOptions{Fields: []string{"bukrs", "ACTVT", "zaction"}})
	if err != nil {
		t.Fatal(err)
	}
	wellFormed(t, s)
	if strings.Count(s, "<suso:authField>") != 3 || !strings.Contains(s, "<suso:name>BUKRS</suso:name>") || strings.Count(s, "<suso:activity>") != 4 {
		t.Fatalf("%s", s)
	}
	if _, err := buildAuthObjectContent(AuthObjectOptions{NoActivity: true}); err == nil {
		t.Fatal("no field accepted")
	}
	if _, err := buildAuthObjectContent(AuthObjectOptions{Fields: []string{"X"}, Activities: []string{"1"}}); err == nil {
		t.Fatal("activity 1 accepted")
	}
}

func TestAuthFieldAndScenarioContent(t *testing.T) {
	s, err := buildAuthFieldContent("ZF", "zde_x")
	if err != nil || !strings.Contains(s, "<auth:rollName>ZDE_X</auth:rollName>") {
		t.Fatalf("%s %v", s, err)
	}
	wellFormed(t, s)
	if _, err := buildAuthFieldContent("ZF", ""); err == nil {
		t.Fatal("no data element accepted")
	}
	c := buildCommScenarioContent("ZCS_X", []string{"zapi_x_o4_0001_g4ba", ""})
	wellFormed(t, c)
	if !strings.Contains(c, "<sco1:inboundID>0001</sco1:inboundID><sco1:ibsID>ZAPI_X_O4_0001_G4BA</sco1:ibsID><sco1:ibsType>G4BA</sco1:ibsType>") {
		t.Fatalf("%s", c)
	}
}

func TestBAdIImplContent(t *testing.T) {
	s, err := buildBAdIImplContent(BAdIImplOptions{Name: "zei_x", Description: "d", EnhancementSpot: "mmim_cloud_badi",
		BAdIDefinition: "MMIM_ITEM_CHECK_DATA", ImplementingClass: "zcl_x"})
	if err != nil {
		t.Fatal(err)
	}
	wellFormed(t, s)
	for _, want := range []string{`enho:name="ZEI_X"`, `enhsxsb/mmim_cloud_badi#type=enhs%2fxb;name=mmim_item_check_data`, `adtcore:name="ZCL_X"`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in\n%s", want, s)
		}
	}
	if _, err := buildBAdIImplContent(BAdIImplOptions{Name: "Z"}); err == nil {
		t.Fatal("missing spot accepted")
	}
}

func TestXMLTypesRegistered(t *testing.T) {
	for ot, want := range map[CreatableObjectType]string{ObjectTypeAuthField: "/sap/bc/adt/aps/iam/auth/zf",
		ObjectTypeAuthObject: "/sap/bc/adt/aps/iam/suso/zf", ObjectTypeCommScenario: "/sap/bc/adt/aps/cloud/com/sco1/zf",
		ObjectTypeBAdIImpl: "/sap/bc/adt/enhancements/enhoxhb/zf"} {
		if fisObjectURL(ot, "ZF") != want {
			t.Errorf("%s url %s", ot, fisObjectURL(ot, "ZF"))
		}
		info, ok := objectTypes[ot]
		if !ok || info.bodyBuilder == nil {
			t.Fatalf("%s not registered", ot)
		}
		body := info.bodyBuilder(CreateObjectOptions{ObjectType: ot, Name: "zf", Description: "a&b", PackageName: "ytest", Source: "  <x/>"}, info, "U")
		if !strings.Contains(body, `adtcore:description="a&amp;b"`) || !strings.Contains(body, `adtcore:name="YTEST"`) {
			t.Errorf("%s body:\n%s", ot, body)
		}
		if err := xml.Unmarshal([]byte(strings.Replace(body, "<x/>", "", 1)), new(struct{})); err != nil {
			t.Errorf("%s body not XML: %v", ot, err)
		}
	}
	if _, err := (&Client{}).CreateAuthorizationObject(context.Background(), AuthObjectOptions{Name: "ZTOOLONGNAME", Description: "d", Fields: []string{"X"}}); err == nil {
		t.Fatal("11 chars accepted")
	}
}
