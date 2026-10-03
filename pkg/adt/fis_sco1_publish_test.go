package adt

import "testing"

func TestPublishStatusFrom(t *testing.T) {
	for body, want := range map[string]string{
		`<com:publishing xmlns:com="x"><com:statusText>p</com:statusText></com:publishing>`:       "p",
		`<asx:abap><asx:values><DATA><STATUS_TEXT>u</STATUS_TEXT></DATA></asx:values></asx:abap>`: "u",
		`<x:publishingStatusText> p </x:publishingStatusText>`:                                    "p",
		`<nothing/>`: "",
	} {
		if got := publishStatusFrom(body); got != want {
			t.Errorf("%s: got %q want %q", body, got, want)
		}
	}
}
