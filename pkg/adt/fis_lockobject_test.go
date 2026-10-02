package adt

import (
	"strings"
	"testing"
)

func TestBuildLockObjectContent(t *testing.T) {
	got, err := buildLockObjectContent(LockObjectOptions{Table: "ztb_ap_paydoc", Fields: []string{"client", "doc_id"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<enqu:tableName>ZTB_AP_PAYDOC</enqu:tableName>", "<enqu:lockMode>E</enqu:lockMode>",
		"<enqu:parameterName>CLIENT</enqu:parameterName>", "<enqu:fieldName>DOC_ID</enqu:fieldName>", "<enqu:allowRFC>false</enqu:allowRFC>"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in\n%s", want, got)
		}
	}
	if _, err := buildLockObjectContent(LockObjectOptions{Table: "EKKO", Fields: []string{"EBELN"}}); err == nil {
		t.Error("SAP table must be refused")
	}
	if _, err := buildLockObjectContent(LockObjectOptions{Table: "ZTB_X", Fields: []string{"ID"}, LockMode: "Q"}); err == nil {
		t.Error("bad lock mode must be refused")
	}
	if _, err := buildLockObjectContent(LockObjectOptions{Table: "ZTB_X"}); err == nil {
		t.Error("missing fields must be refused")
	}
}
