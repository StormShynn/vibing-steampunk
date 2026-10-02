package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
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
	ObjectTypeLockObject   CreatableObjectType = "ENQU/DL"
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
	// ENQU: document shape read from HL8 EMEKKOE (2026-10-02), media type application/vnd.sap.adt.lockobjects.v1+xml.
	ObjectTypeLockObject: {"/sap/bc/adt/ddic/lockobjects/sources", "enqu:lockobject", `xmlns:enqu="http://www.sap.com/adt/ddic/enqu"`, "ENQU/DL", "", ""},
	ObjectTypeBAdIImpl: {"/sap/bc/adt/enhancements/enhoxhb", "enho:objectData", `xmlns:enho="http://www.sap.com/adt/enhancements/enho"`, "ENHO/XHB",
		` xmlns:enhcore="http://www.sap.com/abapsource/enhancementscore"`,
		""},
}

func init() {
	for ot, t := range fisXMLTypes {
		t := t
		info := objectTypeInfo{creationPath: t.collection, rootName: t.root, namespace: t.ns}
		if ot == ObjectTypeBAdIImpl {
			info.contentType = "application/vnd.sap.adt.enh.enhoxhb.v4+xml"
		}
		if ot == ObjectTypeLockObject {
			info.contentType = "application/vnd.sap.adt.lockobjects.v1+xml"
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

// buildBAdIImplShell is the create body Eclipse ADT sends (HL8 ADT
// communication log, 2026-09-28): the spot as an EXTO usage and an EMPTY list
// of BAdI implementations. Posting the implementation in the create body is
// what SAP refuses with "No documentation class is assigned to object R3TR
// ENHO" (SD 269); the implementation is added by a PUT afterwards.
func buildBAdIImplShell(o BAdIImplOptions) (string, error) {
	spot := strings.ToUpper(strings.TrimSpace(o.EnhancementSpot))
	if spot == "" || strings.TrimSpace(o.BAdIDefinition) == "" || strings.TrimSpace(o.ImplementingClass) == "" {
		return "", fmt.Errorf("enhancement_spot, badi_definition and implementing_class are required")
	}
	return fmt.Sprintf(`  <enho:contentCommon enho:toolType="BADI_IMPL">
    <enho:usages>
      <enhcore:referencedObject enhcore:element_usage="EXTO" enhcore:program_id="R3TR">
        <enhcore:objectReference adtcore:name="%s" adtcore:type="ENHS/XS"/>
        <enhcore:mainObjectReference/>
      </enhcore:referencedObject>
    </enho:usages>
  </enho:contentCommon>
  <enho:contentSpecific>
    <enho:badiTechnology>
      <enho:badiImplementations/>
    </enho:badiTechnology>
  </enho:contentSpecific>`, escapeXML(spot)), nil
}

// buildBAdIImplElement is one enho:badiImplementation, in the shape SAP serves
// for an existing implementation (ZEI_MMIM_SLOC_CHECK).
func buildBAdIImplElement(o BAdIImplOptions) (string, error) {
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
	return fmt.Sprintf(`<enho:badiImplementation enho:name="%s" enho:shortText="%s" enho:example="%t" enho:default="%t" enho:active="%t"><enho:enhancementSpot adtcore:uri="/sap/bc/adt/enhancements/enhsxsb/%s" adtcore:type="ENHS/XSB" adtcore:name="%s"/><enho:badiDefinition adtcore:uri="/sap/bc/adt/enhancements/enhsxsb/%s#type=enhs%%2fxb;name=%s" adtcore:type="ENHS/XB" adtcore:name="%s"/><enho:implementingClass adtcore:uri="/sap/bc/adt/oo/classes/%s" adtcore:type="CLAS/OC" adtcore:name="%s"/></enho:badiImplementation>`,
		escapeXML(impl), escapeXML(short), o.Example, o.Default, o.Active, ls, spot, ls, ld, def, lc, cls), nil
}

// buildBAdIImplContent is kept for PlanBAdIImplementation's validation.
func buildBAdIImplContent(o BAdIImplOptions) (string, error) {
	if _, err := buildBAdIImplShell(o); err != nil {
		return "", err
	}
	return buildBAdIImplElement(o)
}

// insertBAdIImpl adds one badiImplementation to the server's ENHO document.
func insertBAdIImpl(doc, elem string) (string, error) {
	if i := strings.Index(doc, "<enho:badiImplementations/>"); i >= 0 {
		return doc[:i] + "<enho:badiImplementations>" + elem + "</enho:badiImplementations>" + doc[i+len("<enho:badiImplementations/>"):], nil
	}
	if i := strings.Index(doc, "</enho:badiImplementations>"); i >= 0 {
		return doc[:i] + elem + doc[i:], nil
	}
	// A fresh shell comes back with the list collapsed (HL8, 2026-09-28).
	list := "<enho:badiImplementations>" + elem + "</enho:badiImplementations>"
	for _, empty := range []struct{ tag, repl string }{
		{"<enho:badiTechnology/>", "<enho:badiTechnology>" + list + "</enho:badiTechnology>"},
		{"<enho:contentSpecific/>", "<enho:contentSpecific><enho:badiTechnology>" + list + "</enho:badiTechnology></enho:contentSpecific>"},
	} {
		if i := strings.Index(doc, empty.tag); i >= 0 {
			return doc[:i] + empty.repl + doc[i+len(empty.tag):], nil
		}
	}
	if i := strings.Index(doc, "</enho:badiTechnology>"); i >= 0 {
		return doc[:i] + list + doc[i:], nil
	}
	return "", fmt.Errorf("enho:badiImplementations not found in the server document")
}

const enhoContentType = "application/vnd.sap.adt.enh.enhoxhb.v4+xml"

// CreateBAdIImplementation creates the enhancement implementation shell the way
// Eclipse does, adds the BAdI implementation (lock, GET, PUT, unlock) and activates.
func (c *Client) CreateBAdIImplementation(ctx context.Context, o BAdIImplOptions) (string, error) {
	name, err := checkFISName("CreateBAdIImplementation", o.Name, 30)
	if err != nil {
		return "", err
	}
	shell, err := buildBAdIImplShell(o)
	if err != nil {
		return "", fmt.Errorf("CreateBAdIImplementation: %w", err)
	}
	elem, err := buildBAdIImplElement(o)
	if err != nil {
		return "", fmt.Errorf("CreateBAdIImplementation: %w", err)
	}
	if strings.TrimSpace(o.Description) == "" || len([]rune(o.Description)) > 60 {
		return "", fmt.Errorf("CreateBAdIImplementation: description is required, max 60 characters")
	}
	if err := c.CreateObject(ctx, CreateObjectOptions{ObjectType: ObjectTypeBAdIImpl, Name: name, Description: o.Description,
		PackageName: o.Package, Transport: o.Transport, Source: shell}); err != nil {
		return "", fmt.Errorf("CreateBAdIImplementation: %w", err)
	}
	objURL := fisXMLObjectURL(ObjectTypeBAdIImpl, name)
	ctx = withMutationPackageChecked(ctx, objURL)
	lock, err := c.LockObject(ctx, objURL, "MODIFY")
	if err != nil {
		return objURL, fmt.Errorf("CreateBAdIImplementation: %s created (empty), lock to add the BAdI implementation failed: %w", name, err)
	}
	werr := func() error {
		resp, err := c.transport.Request(ctx, objURL, &RequestOptions{Method: http.MethodGet,
			Accept: "application/vnd.sap.adt.enh.enhoxhb.v3+xml, " + enhoContentType, Stateful: true})
		if err != nil {
			return fmt.Errorf("reading %s: %w", name, err)
		}
		doc, err := insertBAdIImpl(string(resp.Body), elem)
		if err != nil {
			return err
		}
		q := url.Values{}
		q.Set("lockHandle", lock.LockHandle)
		if o.Transport != "" {
			q.Set("corrNr", o.Transport)
		}
		if _, err := c.transport.Request(ctx, objURL, &RequestOptions{Method: http.MethodPut, Query: q, Body: []byte(doc),
			ContentType: enhoContentType, Accept: enhoContentType, Stateful: true}); err != nil {
			return fmt.Errorf("writing the BAdI implementation: %w", err)
		}
		return nil
	}()
	if uerr := c.UnlockObject(ctx, objURL, lock.LockHandle); uerr != nil && werr == nil {
		werr = fmt.Errorf("unlocking: %w", uerr)
	}
	if werr != nil {
		return objURL, fmt.Errorf("CreateBAdIImplementation: %s created (empty): %w", name, werr)
	}
	res, err := c.Activate(ctx, objURL, name)
	if err == nil && !res.Success {
		res, err = c.Activate(ctx, objURL, name)
	}
	if err != nil {
		return objURL, fmt.Errorf("CreateBAdIImplementation: %s created, activation: %w", name, err)
	}
	if !res.Success {
		return objURL, fmt.Errorf("CreateBAdIImplementation: %s created but did not activate: %s", name, strings.Join(res.ProblemLines(), "; "))
	}
	return objURL, nil
}

// PlanBAdIImplementation validates a BAdI implementation without writing and
// returns the Eclipse ADT steps (see handleCreateBAdIImplementation).
func (c *Client) PlanBAdIImplementation(ctx context.Context, o BAdIImplOptions) (string, error) {
	name, err := checkFISName("CreateBAdIImplementation", o.Name, 30)
	if err != nil {
		return "", err
	}
	if err := checkNamingRule("ENHO/XHB", name); err != nil {
		return "", err
	}
	if _, err := buildBAdIImplContent(o); err != nil {
		return "", fmt.Errorf("CreateBAdIImplementation: %w", err)
	}
	cls := strings.ToUpper(strings.TrimSpace(o.ImplementingClass))
	clsState := "found"
	if hits, serr := c.SearchObject(ctx, cls, 5); serr != nil {
		clsState = "not checked (" + serr.Error() + ")"
	} else {
		clsState = "NOT FOUND — create it first (INTERFACES if_badi_interface + the BAdI interface)"
		for _, h := range hits {
			if strings.EqualFold(h.Name, cls) {
				clsState = "found"
				break
			}
		}
	}
	impl := strings.ToUpper(strings.TrimSpace(o.ImplementationName))
	if impl == "" {
		impl = name
	}
	pkg := strings.ToUpper(strings.TrimSpace(o.Package))
	return fmt.Sprintf(`Nothing was written. Create the BAdI implementation in Eclipse ADT:
1. Project Explorer: package %s -> New -> Other ABAP Repository Object -> Enhancements -> BAdI Enhancement Implementation.
2. Name %s, description %q, enhancement spot %s, transport %s.
3. In the editor: Add BAdI Implementation -> BAdI definition %s, implementation name %s, implementing class %s (class: %s).
4. Leave "Implementation is active" %s; example=%t, default=%t. Save and activate.
(Without plan_only the tool creates and activates it itself — verified on HL8, 2026-09-28.)`,
		pkg, name, o.Description, strings.ToUpper(o.EnhancementSpot), o.Transport,
		strings.ToUpper(o.BAdIDefinition), impl, cls, clsState, map[bool]string{true: "ON (user asked)", false: "OFF"}[o.Active], o.Example, o.Default), nil
}
