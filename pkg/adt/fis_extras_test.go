package adt

import "testing"

func TestNormalizeADTReadPath(t *testing.T) {
	ok := []string{
		"/sap/bc/adt/applicationjob/catalogs/zjob_einv",
		"/sap/bc/adt/applicationjob/templates/ZJOB_TPL_EINV/",
		"/sap/bc/adt/aps/cloud/iam/sia6/zjob_einv_sajc",
		"/sap/bc/adt/ddic/domains/abap_boolean?version=active",
	}
	for _, p := range ok {
		if _, err := normalizeADTReadPath(p); err != nil {
			t.Errorf("%s: unexpected error %v", p, err)
		}
	}
	bad := []string{
		"", "https://evil.example/sap/bc/adt/ddic/x", "/sap/bc/adt/oo/classes/zcl_x",
		"/sap/bc/adt/ddic", "/sap/bc/adt/ddic/../oo/classes/x", "/sap/bc/adt/ddic//x",
		"/sap/bc/adt/discovery", "sap/bc/adt/ddic/x",
	}
	for _, p := range bad {
		if _, err := normalizeADTReadPath(p); err == nil {
			t.Errorf("%s: expected an error", p)
		}
	}
}

func TestClassRunName(t *testing.T) {
	for _, n := range []string{"ZCL_FISST_SETUP", "/ABC/CL_X", "YCL_A"} {
		if !classRunName.MatchString(n) {
			t.Errorf("%s should be valid", n)
		}
	}
	for _, n := range []string{"", "zcl lower space", "ZCL_X;DROP", "ZCL_ABCDEFGHIJKLMNOPQRSTUVWXYZ12345"} {
		if classRunName.MatchString(n) {
			t.Errorf("%s should be invalid", n)
		}
	}
}

func TestParsePackageNodeStructureSkipsErrorNodes(t *testing.T) {
	xmlDoc := []byte(`<asx:abap xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA><TREE_CONTENT>` +
		`<SEU_ADT_REPOSITORY_OBJ_NODE><OBJECT_TYPE></OBJECT_TYPE><OBJECT_NAME>Error loading node:</OBJECT_NAME><DESCRIPTION>CHDO ZFISPCE</DESCRIPTION></SEU_ADT_REPOSITORY_OBJ_NODE>` +
		`<SEU_ADT_REPOSITORY_OBJ_NODE><OBJECT_TYPE>CLAS/OC</OBJECT_TYPE><OBJECT_NAME>ZCL_A</OBJECT_NAME><OBJECT_URI>/sap/bc/adt/oo/classes/zcl_a</OBJECT_URI><DESCRIPTION>A</DESCRIPTION></SEU_ADT_REPOSITORY_OBJ_NODE>` +
		`</TREE_CONTENT></DATA></asx:values></asx:abap>`)
	pkg, err := parsePackageNodeStructure(xmlDoc, "ZP")
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.Objects) != 1 || pkg.Objects[0].Name != "ZCL_A" || pkg.Objects[0].Description != "A" {
		t.Fatalf("objects = %+v", pkg.Objects)
	}
	if len(pkg.Warnings) != 1 {
		t.Fatalf("warnings = %v", pkg.Warnings)
	}
}
