package adt

// FIS fix-8u: "Publish Locally" for a customer communication scenario (SCO1).
//
// Read from HL8 (2026-10-03): CL_APS_COM_WBI_SCO1_WB_ACCESS registers
//
//	relation "publish", template /aps/cloud/com/sco1/$publish{?name},
//	handler CL_APS_COM_WBI_SCO1_PUBLISH, content application/vnd.sap.adt.com.publishing+xml
//
// and the handler's POST reads the MANDATORY query parameter
// communicationScenarioID (not "name"), refuses a request without an Accept
// header, activates the scenario (wait 100 s) and answers the publishing status
// (cl_aps_cob_wbi_utils=>ty_publishing_status-status_text, "p" / "u").

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const CommScenarioPublishContentType = "application/vnd.sap.adt.com.publishing+xml"

var reStatusText = regexp.MustCompile(`(?is)<([A-Za-z0-9_]+:)?(publishingStatusText|statusText|status_text|STATUS_TEXT)>([^<]*)<`)

// publishStatusFrom picks the status text out of the handler's answer.
func publishStatusFrom(body string) string {
	if m := reStatusText.FindStringSubmatch(body); m != nil {
		return strings.TrimSpace(m[3])
	}
	return ""
}

// PublishCommunicationScenario publishes a communication scenario locally so an
// admin can create a communication arrangement for it. Returns the status
// SAP reports ("p" published, "u" not yet) and the raw answer for diagnosis.
func (c *Client) PublishCommunicationScenario(ctx context.Context, name string) (status, raw string, err error) {
	n, err := checkFISName("PublishCommunicationScenario", name, 30)
	if err != nil {
		return "", "", err
	}
	if err := c.checkMutation(ctx, MutationContext{
		Op:        OpUpdate,
		OpName:    "PublishCommunicationScenario",
		ObjectURL: fisXMLObjectURL(ObjectTypeCommScenario, n),
	}); err != nil {
		return "", "", err
	}
	params := url.Values{}
	params.Set("communicationScenarioID", n)
	params.Set("name", n)
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/aps/cloud/com/sco1/$publish", &RequestOptions{
		Method:      http.MethodPost,
		Query:       params,
		ContentType: CommScenarioPublishContentType,
		Accept:      CommScenarioPublishContentType,
	})
	if err != nil {
		return "", "", fmt.Errorf("publishing communication scenario %s: %w", n, err)
	}
	raw = string(resp.Body)
	return publishStatusFrom(raw), raw, nil
}
