package adt

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNormalizeServerDrivenJSON(t *testing.T) {
	b, err := normalizeServerDrivenJSON(`{"subobjects":[{"name":"LOG","description":"x"}]}`, "Log", "en")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	h := m["header"].(map[string]any)
	if m["formatVersion"] != "1" || h["description"] != "Log" || h["originalLanguage"] != "en" || h["abapLanguageVersion"] != "cloudDevelopment" {
		t.Fatalf("bad header: %s", b)
	}
	b, _ = normalizeServerDrivenJSON(`{"header":{"description":"keep"}}`, "Other", "en")
	if !strings.Contains(string(b), `"keep"`) || strings.Contains(string(b), "Other") {
		t.Fatalf("overwrote header: %s", b)
	}
	if _, err := normalizeServerDrivenJSON(`[1]`, "d", "en"); err == nil {
		t.Fatal("array accepted")
	}
}

func TestBuildNumberRangeJSON(t *testing.T) {
	s, err := buildNumberRangeJSON(NumberRangeObjectOptions{Domain: "zdo_x"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]map[string]any
	json.Unmarshal([]byte(s), &m)
	iv, cf := m["interval"], m["configuration"]
	if iv["numberLengthDomain"] != "ZDO_X" || iv["percentWarning"] != 10.0 || iv["rolling"] != false || iv["prefix"] != false || iv["subType"] != "" {
		t.Fatalf("interval %v", iv)
	}
	if cf["buffering"] != "mainBuffer" || cf["bufferedNumbers"] != 1.0 {
		t.Fatalf("configuration %v", cf)
	}
	if _, err := buildNumberRangeJSON(NumberRangeObjectOptions{}); err == nil {
		t.Fatal("no domain accepted")
	}
	if _, err := buildNumberRangeJSON(NumberRangeObjectOptions{Domain: "D", PercentWarning: 150}); err == nil {
		t.Fatal("150% accepted")
	}
}

func TestBuildApplicationLogJSON(t *testing.T) {
	s, err := buildApplicationLogJSON([]LogSubobject{{Name: "paysend", Description: "Gui"}})
	if err != nil || s != `{"subobjects":[{"name":"PAYSEND","description":"Gui"}]}` {
		t.Fatalf("%s %v", s, err)
	}
	if _, err := buildApplicationLogJSON([]LogSubobject{{Name: "", Description: "x"}}); err == nil {
		t.Fatal("empty name accepted")
	}
}

func TestCreateServerDrivenObjectValidation(t *testing.T) {
	c := &Client{}
	ctx := context.Background()
	if _, err := c.CreateServerDrivenObject(ctx, ServerDrivenOptions{Type: "PROG", Name: "ZX", Description: "d"}); err == nil {
		t.Fatal("PROG accepted")
	}
	if _, err := c.CreateServerDrivenObject(ctx, ServerDrivenOptions{Type: "NROB", Name: "ZTOOLONGNAME1", Description: "d"}); err == nil {
		t.Fatal("NROB 13 chars accepted")
	}
	if _, err := c.CreateServerDrivenObject(ctx, ServerDrivenOptions{Type: "nrob", Name: "AX", Description: "d"}); err == nil {
		t.Fatal("non Z/Y accepted")
	}
	if _, err := c.CreateNumberRangeObject(ctx, NumberRangeObjectOptions{Name: "ZFIS_NR01", Description: "d"}); err == nil {
		t.Fatal("number range without domain accepted")
	}
}

func TestIsMediaTypeRejection(t *testing.T) {
	if !isMediaTypeRejection(errors.New("ADT API error: status 415 at /x")) || !isMediaTypeRejection(errors.New("status 406: not acceptable")) {
		t.Fatal("415/406 not detected")
	}
	if isMediaTypeRejection(errors.New("status 400: already exists")) || isMediaTypeRejection(nil) {
		t.Fatal("false positive")
	}
}

func TestFISSourceTypes(t *testing.T) {
	cases := map[CreatableObjectType]string{
		ObjectTypeDDLX: "/sap/bc/adt/ddic/ddlx/sources/zc_x", ObjectTypeDCLS: "/sap/bc/adt/acm/dcl/sources/zc_x",
		ObjectTypeStructure: "/sap/bc/adt/ddic/structures/zc_x", ObjectTypeMessageClass: "/sap/bc/adt/messageclass/zc_x",
	}
	for ty, want := range cases {
		if got := fisObjectURL(ty, "ZC_X"); got != want {
			t.Errorf("%s: %s want %s", ty, got, want)
		}
		if _, ok := objectTypes[ty]; !ok {
			t.Errorf("%s not registered for CreateObject", ty)
		}
	}
	if fisObjectURL("CLAS/OC", "X") != "" {
		t.Error("class should not map")
	}
	for _, code := range []string{"ddlx", "DCLS", "STRU"} {
		if _, ok := fisSourceTypeFor(code); !ok {
			t.Errorf("%s missing", code)
		}
	}
	if objectTypes[ObjectTypeStructure].contentType != "application/vnd.sap.adt.structures.v2+xml" {
		t.Error("structure content type")
	}
}

func TestCreateMessageClassValidation(t *testing.T) {
	c := &Client{}
	ctx := context.Background()
	if _, err := c.CreateMessageClass(ctx, MessageClassOptions{Name: "ZMS_AP", Description: "d", Messages: []MessageClassMessage{{Number: "1", Text: "x"}}}); err == nil {
		t.Fatal("number 1 accepted")
	}
	if _, err := c.CreateMessageClass(ctx, MessageClassOptions{Name: "MS_AP", Description: "d"}); err == nil {
		t.Fatal("non Z accepted")
	}
	if _, err := c.CreateMessageClass(ctx, MessageClassOptions{Name: "ZMS_AP", Description: ""}); err == nil {
		t.Fatal("empty description accepted")
	}
}
