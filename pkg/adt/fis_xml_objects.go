package adt

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// --- FIS: XML-form objects on S/4HANA Cloud Public Edition ----------------
//
// Authorization fields (AUTH), authorization objects (SUSO), communication
// scenarios (SCO1) and BAdI implementations (ENHO/XHB) are plain ADT XML
// documents (not the blue + JSON form of NROB/APLO). Document shapes read from
// HL8 on 2026-09-27: AUTH ZACTION, SUSO ZAU_APPRPP, SCO1 ZCS_DEMO_PC, ENHO
// ZEI_MMIM_SLOC_CHECK. Only the identifying subset is written on create — the
// workbench fills the rest (visibility flags, plugin configs) itself, as it
// does for SIA6.
//
// The inner <content> is prepared by the typed Create* function and carried to
// the body builder in CreateObjectOptions.Source.

const (
	ObjectTypeAuthField    CreatableObjectType = "AUTH"
	ObjectTypeAuthObject   CreatableObjectType = "SUSO/B"
	ObjectTypeCommScenario CreatableObjectType = "SCO1"
	ObjectTypeBAdIImpl     CreatableObjectType = "ENHO/XHB"
)

type fisXMLType struct {
	collection string
	root       string
	ns         string
	adtType    string
	extraNS    string
	rootAttrs  string // extra root attributes
}

var fisXMLTypes = map[CreatableObjectType]fisXMLType{
	ObjectTypeAuthField:    {"/sap/bc/adt/aps/iam/auth", "auth:auth", `xmlns:auth="http://www.sap.com/iam/auth"`, "AUTH", "", ""},
	ObjectTypeAuthObject:   {"/sap/bc/adt/aps/iam/suso", "suso:suso", `xmlns:suso="http://www.sap.com/iam/suso"`, "SUSO/B", "", ""},
	ObjectTypeCommScenario: {"/sap/bc/adt/aps/cloud/com/sco1", "sco1:sco1", `xmlns:sco1="http://www.sap.com/com/sco1"`, "SCO1", "", ""},
	ObjectTypeBAdIImpl: {"/sap/bc/adt/enhancements/enhoxhb", "enho:objectData", `xmlns:enho="http://www.sap.com/adt/enhancements/enho"`, "ENHO/XHB",
		` xmlns:enhcore="http://www.sap.com/abapsource/enhancementscore"`,
		// ZEI_MMIM_SLOC_CHECK (HL8) carries it; without it the POST fails with
		// "No documentation class is assigned to object R3TR ENHO" [Inference].
		"\n  adtcore:abapLanguageVersion=\"cloudDevelopment\""},
}

func init() {
	for ot, t := range fisXMLTypes {
		t := t
		info := objectTypeInfo{creationPath: t.collection, rootName: t.root, namespace: t.ns}
		if ot == ObjectTypeBAdIImpl {
			info.contentType = "application/vnd.sap.adt.enh.enhoxhb.v4+xml"
		}
		info.bodyBuilder = func(opts CreateObjectOptions, ti objectTypeInfo, responsible string) string {
			return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<%s %s%s xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:description="%s"
  adtcore:name="%s"
  adtcore:type="%s"
  adtcore:responsible="%s"
  adtcore:masterLanguage="EN"%s
  adtcore:language="EN">
  <adtcore:packageRef adtcore:name="%s"/>
%s
</%s>`, t.root, t.ns, t.extraNS, escapeXML(opts.Description), strings.ToUpper(opts.Name), t.adtType, responsible, t.rootAttrs,
				strings.ToUpper(opts.PackageName), opts.Source, t.root)
		}
		objectTypes[ot] = info
	}
}

// fisXMLObjectURL returns the object URL of an XML-form FIS type ("" if not one).
func fisXMLObjectURL(ot CreatableObjectType, name string) string {
	if t, ok := fisXMLTypes[ot]; ok {
		return t.collection + "/" + strings.ToLower(name)
	}
	return ""
}

var fisXMLName = regexp.MustCompile(`^[ZY][A-Z0-9_]*$`)

func checkFISName(op, name string, max int) (string, error) {
	n := strings.ToUpper(strings.TrimSpace(name))
	if !fisXMLName.MatchString(n) || len(n) > max {
		return "", fmt.Errorf("%s: name %q must start with Z or Y, A-Z 0-9 _ only, max %d", op, name, max)
	}
	return n, nil
}

// createAndActivateXML creates an XML-form object and activates it (retrying once).
func (c *Client) createAndActivateXML(ctx context.Context, op string, ot CreatableObjectType, name, description, pkg, transport, content string) (string, error) {
	if strings.TrimSpace(description) == "" || len([]rune(description)) > 60 {
		return "", fmt.Errorf("%s: description is required, max 60 characters", op)
	}
	if err := c.CreateObject(ctx, CreateObjectOptions{ObjectType: ot, Name: name, Description: description,
		PackageName: pkg, Transport: transport, Source: content}); err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}
	objURL := fisXMLObjectURL(ot, name)
	res, err := c.Activate(ctx, objURL, name)
	if err == nil && !res.Success {
		res, err = c.Activate(ctx, objURL, name)
	}
	if err != nil {
		return objURL, fmt.Errorf("%s: %s created, activation: %w", op, name, err)
	}
	if !res.Success {
		// Authorization fields (and other IAM form objects) are saved active
		// by the POST itself; ADT then refuses a separate activation without a
		// message (HL8, 2026-09-28). Trust the document when it is not inactive.
		// (ProblemLines is never empty: it adds its own "SAP named no reason".)
		if len(res.ErrorMessages()) == 0 && c.xmlObjectNotInactive(ctx, objURL) {
			return objURL, nil
		}
		return objURL, fmt.Errorf("%s: %s created but did not activate: %s (check GetInactiveObjects — SAP sometimes reports a false failure)", op, name, strings.Join(res.ProblemLines(), "; "))
	}
	return objURL, nil
}

// xmlObjectNotInactive reports whether the object's ADT document exists and is not flagged inactive.
func (c *Client) xmlObjectNotInactive(ctx context.Context, objURL string) bool {
	resp, err := c.transport.Request(ctx, objURL, &RequestOptions{Method: http.MethodGet, Accept: "*/*"})
	if err != nil {
		return false
	}
	return !strings.Contains(string(resp.Body), `adtcore:version="inactive"`)
}

// AuthFieldOptions describes an authorization field (AUTH).
type AuthFieldOptions struct {
	Name, Description, Package, Transport string
	DataElement                           string // data element typing the field
}

func buildAuthFieldContent(name, dtel string) (string, error) {
	d := strings.ToUpper(strings.TrimSpace(dtel))
	if d == "" {
		return "", fmt.Errorf("data_element is required")
	}
	return fmt.Sprintf(`  <auth:content>
    <auth:fieldName>%s</auth:fieldName>
    <auth:rollName>%s</auth:rollName>
    <auth:checkTable/>
    <auth:exitFB/>
    <auth:search>false</auth:search>
    <auth:objexit>false</auth:objexit>
  </auth:content>`, escapeXML(name), escapeXML(d)), nil
}

// CreateAuthorizationField creates and activates an authorization field.
func (c *Client) CreateAuthorizationField(ctx context.Context, o AuthFieldOptions) (string, error) {
	name, err := checkFISName("CreateAuthorizationField", o.Name, 10)
	if err != nil {
		return "", err
	}
	content, err := buildAuthFieldContent(name, o.DataElement)
	if err != nil {
		return "", fmt.Errorf("CreateAuthorizationField: %w", err)
	}
	return c.createAndActivateXML(ctx, "CreateAuthorizationField", ObjectTypeAuthField, name, o.Description, o.Package, o.Transport, content)
}

// AuthObjectOptions describes an authorization object (SUSO).
type AuthObjectOptions struct {
	Name, Description, Package, Transport string
	Fields                                []string // authorization fields besides ACTVT
	Activities                            []string // ACTVT codes, default 01 02 03 06
	NoActivity                            bool     // object without ACTVT
}

var activityText = map[string]string{"01": "Create or generate", "02": "Change", "03": "Display", "06": "Delete",
	"16": "Execute", "05": "Lock", "43": "Release", "A9": "Send"}

func buildAuthObjectContent(o AuthObjectOptions) (string, error) {
	var b strings.Builder
	b.WriteString("  <suso:content>\n    <suso:objectClassName>CPAE</suso:objectClassName>\n    <suso:authFields>\n")
	n := 0
	if !o.NoActivity {
		b.WriteString("      <suso:authField><suso:name>ACTVT</suso:name><suso:activityField>true</suso:activityField></suso:authField>\n")
		n++
	}
	for _, f := range o.Fields {
		f = strings.ToUpper(strings.TrimSpace(f))
		if f == "" || f == "ACTVT" {
			continue
		}
		b.WriteString(fmt.Sprintf("      <suso:authField><suso:name>%s</suso:name><suso:activityField>false</suso:activityField></suso:authField>\n", escapeXML(f)))
		n++
	}
	if n == 0 {
		return "", fmt.Errorf("an authorization object needs at least one field")
	}
	if n > 10 {
		return "", fmt.Errorf("an authorization object has at most 10 fields")
	}
	b.WriteString("    </suso:authFields>\n    <suso:activities>\n")
	if !o.NoActivity {
		acts := o.Activities
		if len(acts) == 0 {
			acts = []string{"01", "02", "03", "06"}
		}
		for _, a := range acts {
			a = strings.ToUpper(strings.TrimSpace(a))
			if len(a) != 2 {
				return "", fmt.Errorf("activity %q must be 2 characters", a)
			}
			b.WriteString(fmt.Sprintf("      <suso:activity><suso:code>%s</suso:code><suso:text>%s</suso:text></suso:activity>\n", a, escapeXML(activityText[a])))
		}
	}
	b.WriteString("    </suso:activities>\n  </suso:content>")
	return b.String(), nil
}

// CreateAuthorizationObject creates and activates an authorization object.
func (c *Client) CreateAuthorizationObject(ctx context.Context, o AuthObjectOptions) (string, error) {
	name, err := checkFISName("CreateAuthorizationObject", o.Name, 10)
	if err != nil {
		return "", err
	}
	content, err := buildAuthObjectContent(o)
	if err != nil {
		return "", fmt.Errorf("CreateAuthorizationObject: %w", err)
	}
	return c.createAndActivateXML(ctx, "CreateAuthorizationObject", ObjectTypeAuthObject, name, o.Description, o.Package, o.Transport, content)
}

// CommScenarioOptions describes a customer communication scenario (SCO1).
type CommScenarioOptions struct {
	Name, Description, Package, Transport string
	InboundServices                       []string // inbound service IDs, e.g. ZAPI_X_O4_0001_G4BA (generated by publishing an API binding)
}

func ibsTypeOf(id string) (string, string) {
	switch {
	case strings.HasSuffix(id, "_G4BA"):
		return "G4BA", "OData V4"
	case strings.HasSuffix(id, "_IWSG"):
		return "IWSG", "OData V2"
	}
	return "", ""
}

func buildCommScenarioContent(name string, ibs []string) string {
	// Element order is a strict XML sequence (ADT answers 500 "System expected
	// the element …obOAuth2MultiConfig" when one is skipped) — mirrored from
	// ZCS_DEMO_PC on HL8, 2026-09-28, with the wizard's defaults.
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`  <sco1:content>
    <sco1:publishIndicator>false</sco1:publishIndicator>
    <sco1:abapLanguageVersion>5</sco1:abapLanguageVersion>
    <sco1:communicationScenarioID>%s</sco1:communicationScenarioID>
    <sco1:communicationScenarioType>1</sco1:communicationScenarioType>
    <sco1:scopeDependent>false</sco1:scopeDependent>
    <sco1:scopeStatus>3</sco1:scopeStatus>
    <sco1:scopeStatusText/>
    <sco1:containsInbound>%t</sco1:containsInbound>
    <sco1:containsOutbound>false</sco1:containsOutbound>
    <sco1:allowedInstances>1</sco1:allowedInstances>
    <sco1:badiClassDeploy/>
    <sco1:badiClassRuntime/>
    <sco1:allowCreateByKey>false</sco1:allowCreateByKey>
    <sco1:ibBasicAuth>true</sco1:ibBasicAuth>
    <sco1:ibX509Auth>true</sco1:ibX509Auth>
    <sco1:ibOAuth2Auth>false</sco1:ibOAuth2Auth>
    <sco1:ibTrustPseID>SSLC_CUSTOMER_DEFAULT</sco1:ibTrustPseID>
    <sco1:ibSignPseID/>
    <sco1:ibEncryptPseID/>
    <sco1:ibRoleID/>
    <sco1:ibSapManagedUserName/>
    <sco1:dbmsUserRequired>false</sco1:dbmsUserRequired>
    <sco1:obNoneAuth>false</sco1:obNoneAuth>
    <sco1:obBasicAuth>true</sco1:obBasicAuth>
    <sco1:obX509Auth>true</sco1:obX509Auth>
    <sco1:obOAuth1Auth>false</sco1:obOAuth1Auth>
    <sco1:obOAuth2Auth>false</sco1:obOAuth2Auth>
    <sco1:obTrustPseID>SSLC_CUSTOMER_DEFAULT</sco1:obTrustPseID>
    <sco1:obSignPseID/>
    <sco1:obEncryptPseID/>
    <sco1:obAuthPseID>SSLC_CUSTOMER_DEFAULT</sco1:obAuthPseID>
    <sco1:obOAuth2MultiConfig>false</sco1:obOAuth2MultiConfig>
    <sco1:obOAuth2ClientProfile>SAP_APS_COM_A4C_CSCN_SCP</sco1:obOAuth2ClientProfile>
    <sco1:obOAuth2GrantType>0</sco1:obOAuth2GrantType>
    <sco1:obOAuth2TargetPath/>
    <sco1:rootProperties/>
    <sco1:inboundServices>
`, escapeXML(name), len(ibs) > 0))
	n := 0
	for _, id := range ibs {
		id = strings.ToUpper(strings.TrimSpace(id))
		if id == "" {
			continue
		}
		n++
		t, txt := ibsTypeOf(id)
		b.WriteString(fmt.Sprintf("      <sco1:inboundService><sco1:inboundID>%04d</sco1:inboundID><sco1:ibsID>%s</sco1:ibsID><sco1:ibsType>%s</sco1:ibsType><sco1:ibsTypeText>%s</sco1:ibsTypeText><sco1:text/><sco1:isHidden>false</sco1:isHidden><sco1:partnerType/><sco1:partnerRole/><sco1:messageCode/><sco1:messageFunction/><sco1:processCode/><sco1:triggerImmediately>false</sco1:triggerImmediately><sco1:srvcType>HT</sco1:srvcType><sco1:srvcName/></sco1:inboundService>\n",
			n, escapeXML(id), t, txt))
	}
	b.WriteString(`    </sco1:inboundServices>
    <sco1:inboundServiceProperties/>
    <sco1:outboundServices/>
    <sco1:outboundServiceProperties/>
    <commonAuthorization:auths xmlns:commonAuthorization="http://www.sap.com/aps/common/authorization"/>
  </sco1:content>`)
	return b.String()
}

// CreateCommunicationScenario creates a customer communication scenario, optionally with inbound services.
func (c *Client) CreateCommunicationScenario(ctx context.Context, o CommScenarioOptions) (string, error) {
	name, err := checkFISName("CreateCommunicationScenario", o.Name, 30)
	if err != nil {
		return "", err
	}
	return c.createAndActivateXML(ctx, "CreateCommunicationScenario", ObjectTypeCommScenario, name, o.Description, o.Package, o.Transport,
		buildCommScenarioContent(name, o.InboundServices))
}

// BAdIImplOptions describes an enhancement implementation holding one BAdI implementation.
type BAdIImplOptions struct {
	Name, Description, Package, Transport string
	EnhancementSpot                       string // e.g. MMIM_CLOUD_BADI
	BAdIDefinition                        string // e.g. MMIM_ITEM_CHECK_DATA
	ImplementationName                    string // e.g. ZBDI_… (default: Name)
	ImplementingClass                     string // existing class implementing the BAdI interface
	Example, Default                      bool
	Active                                bool // runtime switch of the BAdI implementation; default false (not called) — switch on only when the user asks
}

func buildBAdIImplContent(o BAdIImplOptions) (string, error) {
	spot := strings.ToUpper(strings.TrimSpace(o.EnhancementSpot))
	def := strings.ToUpper(strings.TrimSpace(o.BAdIDefinition))
	cls := strings.ToUpper(strings.TrimSpace(o.ImplementingClass))
	impl := strings.ToUpper(strings.TrimSpace(o.ImplementationName))
	if impl == "" {
		impl = strings.ToUpper(strings.TrimSpace(o.Name))
	}
	if spot == "" || def == "" || cls == "" {
		return "", fmt.Errorf("enhancement_spot, badi_definition and implementing_class are required")
	}
	ls, ld, lc := strings.ToLower(spot), strings.ToLower(def), strings.ToLower(cls)
	short := o.Description
	if len([]rune(short)) > 40 {
		short = string([]rune(short)[:40])
	}
	return fmt.Sprintf(`  <enho:contentCommon enho:toolType="BADI_IMPL"/>
  <enho:contentSpecific>
    <enho:badiTechnology>
      <enho:badiImplementations>
        <enho:badiImplementation enho:name="%s" enho:shortText="%s" enho:example="%t" enho:default="%t" enho:active="%t">
          <enho:enhancementSpot adtcore:uri="/sap/bc/adt/enhancements/enhsxsb/%s" adtcore:type="ENHS/XSB" adtcore:name="%s"/>
          <enho:badiDefinition adtcore:uri="/sap/bc/adt/enhancements/enhsxsb/%s#type=enhs%%2fxb;name=%s" adtcore:type="ENHS/XB" adtcore:name="%s"/>
          <enho:implementingClass adtcore:uri="/sap/bc/adt/oo/classes/%s" adtcore:type="CLAS/OC" adtcore:name="%s"/>
        </enho:badiImplementation>
      </enho:badiImplementations>
    </enho:badiTechnology>
  </enho:contentSpecific>`, escapeXML(impl), escapeXML(short), o.Example, o.Default, o.Active, ls, spot, ls, ld, def, lc, cls), nil
}

// CreateBAdIImplementation creates an enhancement implementation with one BAdI implementation and activates it.
func (c *Client) CreateBAdIImplementation(ctx context.Context, o BAdIImplOptions) (string, error) {
	name, err := checkFISName("CreateBAdIImplementation", o.Name, 30)
	if err != nil {
		return "", err
	}
	content, err := buildBAdIImplContent(o)
	if err != nil {
		return "", fmt.Errorf("CreateBAdIImplementation: %w", err)
	}
	return c.createAndActivateXML(ctx, "CreateBAdIImplementation", ObjectTypeBAdIImpl, name, o.Description, o.Package, o.Transport, content)
}
