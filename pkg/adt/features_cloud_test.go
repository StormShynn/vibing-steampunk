package adt

import (
	"errors"
	"testing"
)

func TestEndpointAnswered(t *testing.T) {
	yes := []error{
		errors.New("ADT API error: status 400 at /sap/bc/adt/ddic/ddl/sources: HTTP&#x20;method&#x20;OPTIONS&#x20;not&#x20;supported"),
		errors.New("ADT API error: status 400 at /sap/bc/adt/cts/transports: HTTP method OPTIONS not supported"),
		errors.New("ADT API error: status 405 at /x"),
	}
	for _, e := range yes {
		if !endpointAnswered(e) {
			t.Errorf("want answered: %v", e)
		}
	}
	no := []error{nil, errors.New("status 404 not found"), errors.New("dial tcp: timeout")}
	for _, e := range no {
		if endpointAnswered(e) {
			t.Errorf("want not answered: %v", e)
		}
	}
}

func TestIsCloudTenant(t *testing.T) {
	for url, want := range map[string]bool{
		"https://my1234567.s4hana.cloud.sap":       true,
		"https://vhcala4hci.dummy.nodomain:44300": false,
	} {
		p := &FeatureProber{client: &Client{config: &Config{BaseURL: url}}}
		if got := p.isCloudTenant(); got != want {
			t.Errorf("%s: got %v want %v", url, got, want)
		}
	}
}
