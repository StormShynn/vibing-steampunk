package adt

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildCatalogSourceMatchesHL8Shape(t *testing.T) {
	b, err := buildCatalogSource("Job entry for E-Invoice", "ZCL_JOB_EINV", "en")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["formatVersion"] != "1" {
		t.Fatalf("formatVersion = %v", m["formatVersion"])
	}
	h := m["header"].(map[string]any)
	if h["description"] != "Job entry for E-Invoice" || h["originalLanguage"] != "en" || h["abapLanguageVersion"] != "cloudDevelopment" {
		t.Fatalf("header = %v", h)
	}
	if m["generalInformation"].(map[string]any)["className"] != "ZCL_JOB_EINV" {
		t.Fatalf("generalInformation = %v", m["generalInformation"])
	}
}

func TestBuildTemplateSource(t *testing.T) {
	b, _ := buildTemplateSource("Job Template", "ZJOB_EINV", "en")
	if !strings.Contains(string(b), `"catalogName": "ZJOB_EINV"`) {
		t.Fatalf("got %s", b)
	}
}

func TestBuildBlueShellEscapes(t *testing.T) {
	s := buildBlueShell("ZJOB_X", "SAJC", `A & "B"`, "ZPK")
	if !strings.Contains(s, `adtcore:type="SAJC"`) || strings.Contains(s, `A & "B"`) || !strings.Contains(s, `adtcore:name="ZPK"`) {
		t.Fatalf("got %s", s)
	}
}

func TestValidateAPJ(t *testing.T) {
	if _, _, err := validateAPJ("x", "zjob_ok", "desc", "zcl_job_ok", "class_name"); err != nil {
		t.Fatalf("valid input refused: %v", err)
	}
	for _, c := range [][3]string{
		{"AJOB", "d", "ZCL_X"}, // not Z/Y
		{"ZJOB", "", "ZCL_X"},  // no description
		{"ZJOB", strings.Repeat("x", 61), "ZCL_X"},
		{"ZJOB", "d", "ZCL X"}, // bad class
		{"ZJOB-1", "d", "ZCL_X"},
	} {
		if _, _, err := validateAPJ("x", c[0], c[1], c[2], "class_name"); err == nil {
			t.Errorf("%v accepted", c)
		}
	}
}
