package adt

// FIS fix-8t: outbound service (SCO3) and outbound services in a communication
// scenario (SCO1). Element order mirrored from HL8 documents (2026-10-03):
// SCO3 CT_SBL_COTA (type COTA) and ZOB_EINVOICE_REST (type REST); SCO1
// ZSCE_EINVOICE (one outbound service).

import (
	"context"
	"fmt"
	"strings"
)

// OutboundServiceOptions describes an outbound service.
type OutboundServiceOptions struct {
	Name, Description, Package, Transport string
	Type                                  string // COTA (default) or REST
	CommunicationTarget                   string // COTA: communication target ID (ZCT_…)
	URLPath                               string // REST: default path, e.g. /api
}

func buildOutboundServiceContent(name string, o OutboundServiceOptions) (string, error) {
	typ := strings.ToUpper(strings.TrimSpace(o.Type))
	if typ == "" {
		typ = "COTA"
	}
	cota, path, comType, comTxt := "", "", "", ""
	switch typ {
	case "COTA":
		cota = strings.ToUpper(strings.TrimSpace(o.CommunicationTarget))
		if !fisXMLName.MatchString(cota) {
			return "", fmt.Errorf("communication_target is required for type COTA (Z/Y communication target, e.g. ZCT_INT_BANK)")
		}
		if !strings.HasSuffix(name, "_COTA") {
			return "", fmt.Errorf("outbound service on a communication target is named <…>_COTA (SAP convention), got %s", name)
		}
		comType, comTxt = "H", "HTTP Service"
	case "REST":
		path = strings.TrimSpace(o.URLPath)
		if path != "" && !strings.HasPrefix(path, "/") {
			return "", fmt.Errorf("url_path must start with /")
		}
	default:
		return "", fmt.Errorf("service_type %q: use COTA (communication target) or REST (HTTP service)", o.Type)
	}
	tag := func(n, v string) string {
		if v == "" {
			return "<sco3:" + n + "/>"
		}
		return "<sco3:" + n + ">" + escapeXML(v) + "</sco3:" + n + ">"
	}
	lv := "5"
	if typ == "COTA" {
		lv = "" // SAP's COTA service shows an empty language version
	}
	return "  <sco3:content>" +
		tag("id", name) + tag("type", typ) + tag("abapLanguageVersion", lv) +
		"<sco3:scopeDependent>false</sco3:scopeDependent>" +
		tag("idocType", "") + tag("idocMsgType", "") + tag("rfcServiceID", "") + tag("leadingBOType", "") +
		"<sco3:publishAPIHub>false</sco3:publishAPIHub>" +
		tag("serviceInterface", "") + tag("desdSchema", "") + tag("cotaId", cota) + tag("urlPath", path) +
		tag("cotaComType", comType) + tag("cotaComTypeTxt", comTxt) +
		"</sco3:content>", nil
}

// CreateOutboundService creates and activates an outbound service (SCO3).
func (c *Client) CreateOutboundService(ctx context.Context, o OutboundServiceOptions) (string, error) {
	name, err := checkFISName("CreateOutboundService", o.Name, 30)
	if err != nil {
		return "", err
	}
	content, err := buildOutboundServiceContent(name, o)
	if err != nil {
		return "", fmt.Errorf("CreateOutboundService: %w", err)
	}
	return c.createAndActivateXML(ctx, "CreateOutboundService", ObjectTypeOutboundSvc, name, o.Description, o.Package, o.Transport, content)
}

func nonEmpty(list []string) []string {
	var out []string
	for _, s := range list {
		if s = strings.ToUpper(strings.TrimSpace(s)); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// buildOutboundServicesXML is the sco1:outboundServices block of a scenario.
func buildOutboundServicesXML(obs []string) string {
	ids := nonEmpty(obs)
	if len(ids) == 0 {
		return "    <sco1:outboundServices/>"
	}
	var b strings.Builder
	b.WriteString("    <sco1:outboundServices>\n")
	for i, id := range ids {
		typ, txt := "REST", "HTTP Service"
		if strings.HasSuffix(id, "_COTA") {
			typ, txt = "COTA", "Communication Target" // text as SAP returns it (HL8 2026-10-03)
		}
		fmt.Fprintf(&b, "      <sco1:outboundService><sco1:outboundID>%04d</sco1:outboundID><sco1:obsID>%s</sco1:obsID><sco1:obsType>%s</sco1:obsType><sco1:obsTypeText>%s</sco1:obsTypeText><sco1:text/><sco1:isMandatory>false</sco1:isMandatory><sco1:partnerType/><sco1:partnerRole/><sco1:messageCode/><sco1:messageFunction/><sco1:processCode/><sco1:supportsPing>false</sco1:supportsPing><sco1:defaultUrl/><sco1:isVirtual>false</sco1:isVirtual><sco1:jobDefinitionName/><sco1:useDefaultLogicalPort>false</sco1:useDefaultLogicalPort><sco1:outputMode/><sco1:receiverPortType/><sco1:packageSize>0000</sco1:packageSize><sco1:queueProcessing>false</sco1:queueProcessing><sco1:usesChangepointer>false</sco1:usesChangepointer><sco1:idocContentType/><sco1:sendDnyEnhSeg>false</sco1:sendDnyEnhSeg><sco1:httpVersion>0</sco1:httpVersion><sco1:httpCompressRequest>0</sco1:httpCompressRequest><sco1:httpCompressReply>false</sco1:httpCompressReply></sco1:outboundService>\n",
			i+1, escapeXML(id), typ, txt)
	}
	b.WriteString("    </sco1:outboundServices>")
	return b.String()
}
