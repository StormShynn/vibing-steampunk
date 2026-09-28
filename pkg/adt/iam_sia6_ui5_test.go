package adt

import (
	"strings"
	"testing"
)

func TestIAMAppBodyUI5AppID(t *testing.T) {
	ti := objectTypes[ObjectTypeIAMApp]
	body := buildIAMAppBody(CreateObjectOptions{
		Name: "ziam_zaa01_ext", Description: "IAM App for ZAA01", PackageName: "zaa01",
		AppType: "EXT", UI5AppID: "zaa01_ui5r",
	}, ti, "CB9980000003")
	want := "<sia6:appType>EXT</sia6:appType>\n    <sia6:ui5AppId>ZAA01_UI5R</sia6:ui5AppId>\n    <sia6:scopeDependent>"
	if !strings.Contains(body, want) {
		t.Fatalf("ui5AppId not written after appType:\n%s", body)
	}
	plain := buildIAMAppBody(CreateObjectOptions{Name: "ZIA_X", Description: "x", PackageName: "Y"}, ti, "U")
	if strings.Contains(plain, "ui5AppId") {
		t.Fatalf("ui5AppId written without UI5AppID:\n%s", plain)
	}
}
