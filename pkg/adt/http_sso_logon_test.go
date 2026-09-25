package adt

import (
	"net/http"
	"testing"
)

func ssoResp(ct string) *http.Response {
	h := http.Header{}
	if ct != "" {
		h.Set("Content-Type", ct)
	}
	return &http.Response{Header: h}
}

func TestServedSSOLogonPage(t *testing.T) {
	saml := `<html><head></head><body onload="document.forms[0].submit()"><form method="POST" action="https://aa1bsvonf.accounts.cloud.sap/saml2/idp/sso/aa1bsvonf.accounts.ondemand.com"><input type="hidden" name="SAMLRequest" value="PHN"></form></body></html>`
	cases := []struct {
		name, ct, body string
		want           bool
	}{
		{"HL8 SAML page", "text/html; charset=utf-8", saml, true},
		{"no content type", "", saml, true},
		{"ABAP source mentioning SAMLRequest", "text/plain", `DATA lv TYPE string VALUE 'name="SAMLRequest"'.`, false},
		{"XML", "application/xml", `<adtcore:objectReferences/>`, false},
		{"plain html page", "text/html", `<html><body>ok</body></html>`, false},
		{"empty", "text/html", ``, false},
	}
	for _, c := range cases {
		if got := servedSSOLogonPage(ssoResp(c.ct), []byte(c.body)); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
