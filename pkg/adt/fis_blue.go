package adt

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// --- FIS: server-driven ("blue" + JSON source) objects -------------------
//
// On S/4HANA Cloud Public Edition several object types are edited in ADT
// through the server-driven form framework: a blue:blueSource shell plus a JSON
// document at <object>/source/main. Read from HL8 (2026-09-27):
//
//	NROB number range object  /sap/bc/adt/numberranges/objects   NROB/NRO
//	  {"formatVersion":"1","header":{…},
//	   "interval":{"numberLengthDomain":"ZDO_…","percentWarning":10.0,"subType":"",
//	               "untilYear":false,"rolling":true,"prefix":false},
//	   "configuration":{"buffering":"mainBuffer","bufferedNumbers":1}}
//	APLO application log obj  /sap/bc/adt/applicationlog/objects APLO/TYP
//	  {"formatVersion":"1","header":{…},"subobjects":[{"name":"LOG","description":"…"}]}
//	SAJC / SAJT               see apj_create.go
//
// A number range object created with CL_NUMBERRANGE_OBJECTS=>create has NO
// object directory entry (ADT answers "Object directory entry was not found"),
// so it is not transportable. Create project number ranges through ADT
// (CreateNumberRangeObject); intervals stay runtime data (CL_NUMBERRANGE_INTERVALS
// or the app Manage Number Range Intervals).

// serverDrivenType is one object type CreateServerDrivenObject may create.
type serverDrivenType struct {
	collection string
	adtType    string
	maxName    int
}

// SUSO / AUTH / SCO1 / ENHO are XML documents, not JSON: see fis_xml_objects.go.
//
// serverDrivenTypes whitelists the collections. Types marked [Unverified] have
// their URI read from HL8 (SearchObject) but their document not yet — they may
// not be JSON-based at all — pass the JSON the
// ADT form editor writes (read an existing object with GetADTObjectXML first).
var serverDrivenTypes = map[string]serverDrivenType{
	"SAJC": {apjCatalogCollection, "SAJC", 30},
	"SAJT": {apjTemplateCollection, "SAJT", 30},
	"NROB": {"/sap/bc/adt/numberranges/objects", "NROB/NRO", 10},
	"APLO": {"/sap/bc/adt/applicationlog/objects", "APLO/TYP", 20},
	"CHDO": {"/sap/bc/adt/changedocuments/objects", "CHDO/CHD", 15}, // [Unverified] JSON
}

// serverDrivenShellTypes: blues v2 (SAJC/SAJT verified), then v1 (what NROB/APLO serve).
var serverDrivenShellTypes = []string{bluesV2ContentType, "application/vnd.sap.adt.blues.v1+xml"}

var sdName = regexp.MustCompile(`^[ZY][A-Z0-9_]*$`)

// isMediaTypeRejection reports whether SAP refused the request's media type.
func isMediaTypeRejection(err error) bool {
	if err == nil {
		return false
	}
	e := strings.ToLower(err.Error())
	return strings.Contains(e, "status 415") || strings.Contains(e, "status 406") ||
		strings.Contains(e, "not acceptable") || strings.Contains(e, "unsupported media type") ||
		strings.Contains(e, "content type")
}

// ServerDrivenOptions describes a server-driven object to create.
type ServerDrivenOptions struct {
	Type        string // key of serverDrivenTypes, e.g. NROB
	Name        string
	Description string
	Package     string
	Transport   string
	JSON        string // content document; header is filled in when missing
}

// normalizeServerDrivenJSON fills formatVersion and header (description,
// originalLanguage, abapLanguageVersion) when the caller left them out.
func normalizeServerDrivenJSON(raw, description, lang string) ([]byte, error) {
	doc := map[string]any{}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			return nil, fmt.Errorf("json_source is not a JSON object: %w", err)
		}
	}
	if _, ok := doc["formatVersion"]; !ok {
		doc["formatVersion"] = "1"
	}
	h, _ := doc["header"].(map[string]any)
	if h == nil {
		h = map[string]any{}
	}
	if _, ok := h["description"]; !ok {
		h["description"] = description
	}
	if _, ok := h["originalLanguage"]; !ok {
		h["originalLanguage"] = lang
	}
	if _, ok := h["abapLanguageVersion"]; !ok {
		h["abapLanguageVersion"] = "cloudDevelopment"
	}
	doc["header"] = h
	return json.MarshalIndent(doc, "", "  ")
}

// CreateServerDrivenObject creates and activates a whitelisted server-driven object.
func (c *Client) CreateServerDrivenObject(ctx context.Context, o ServerDrivenOptions) (string, error) {
	key := strings.ToUpper(strings.TrimSpace(o.Type))
	t, ok := serverDrivenTypes[key]
	if !ok {
		keys := make([]string, 0, len(serverDrivenTypes))
		for k := range serverDrivenTypes {
			keys = append(keys, k)
		}
		return "", fmt.Errorf("CreateServerDrivenObject: type %q not supported (supported: %s)", o.Type, strings.Join(sortedStrings(keys), ", "))
	}
	name := strings.ToUpper(strings.TrimSpace(o.Name))
	if !sdName.MatchString(name) || len(name) > t.maxName {
		return "", fmt.Errorf("CreateServerDrivenObject: %s name %q must start with Z or Y, A-Z 0-9 _ only, max %d", key, o.Name, t.maxName)
	}
	if strings.TrimSpace(o.Description) == "" || len([]rune(o.Description)) > 60 {
		return "", fmt.Errorf("CreateServerDrivenObject: description is required, max 60 characters")
	}
	src, err := normalizeServerDrivenJSON(o.JSON, o.Description, c.apjLanguage())
	if err != nil {
		return "", fmt.Errorf("CreateServerDrivenObject: %w", err)
	}
	return c.blueJSONCreateCT(ctx, "Create"+key, t.collection, t.adtType, name, o.Description, o.Package, o.Transport, src, serverDrivenShellTypes)
}

// NumberRangeObjectOptions describes a number range object (NROB).
type NumberRangeObjectOptions struct {
	Name            string
	Description     string
	Package         string
	Transport       string
	Domain          string  // number length domain, e.g. a CHAR10/NUMC10 domain
	PercentWarning  float64 // default 10
	Rolling         bool
	UntilYear       bool   // intervals per fiscal year
	Buffering       string // mainBuffer (default) | noBuffer | parallelBuffer | localBuffer [Unverified values except mainBuffer]
	BufferedNumbers int    // default 1
}

func buildNumberRangeJSON(o NumberRangeObjectOptions) (string, error) {
	dom := strings.ToUpper(strings.TrimSpace(o.Domain))
	if dom == "" {
		return "", fmt.Errorf("domain (number length domain) is required")
	}
	pw := o.PercentWarning
	if pw <= 0 {
		pw = 10
	}
	if pw > 100 {
		return "", fmt.Errorf("percent_warning must be between 0 and 100")
	}
	buf := o.Buffering
	if buf == "" {
		buf = "mainBuffer"
	}
	bn := o.BufferedNumbers
	if bn <= 0 {
		bn = 1
	}
	doc := map[string]any{
		"interval": map[string]any{
			"numberLengthDomain": dom, "percentWarning": pw, "subType": "",
			"untilYear": o.UntilYear, "rolling": o.Rolling, "prefix": false,
		},
		"configuration": map[string]any{"buffering": buf, "bufferedNumbers": bn},
	}
	b, err := json.Marshal(doc)
	return string(b), err
}

// CreateNumberRangeObject creates a number range object through ADT (with object directory entry).
func (c *Client) CreateNumberRangeObject(ctx context.Context, o NumberRangeObjectOptions) (string, error) {
	js, err := buildNumberRangeJSON(o)
	if err != nil {
		return "", fmt.Errorf("CreateNumberRangeObject: %w", err)
	}
	return c.CreateServerDrivenObject(ctx, ServerDrivenOptions{Type: "NROB", Name: o.Name, Description: o.Description,
		Package: o.Package, Transport: o.Transport, JSON: js})
}

// LogSubobject is one subobject of an application log object.
type LogSubobject struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ApplicationLogObjectOptions describes an application log object (APLO).
type ApplicationLogObjectOptions struct {
	Name        string
	Description string
	Package     string
	Transport   string
	Subobjects  []LogSubobject
}

func buildApplicationLogJSON(subs []LogSubobject) (string, error) {
	out := make([]LogSubobject, 0, len(subs))
	for _, s := range subs {
		n := strings.ToUpper(strings.TrimSpace(s.Name))
		if n == "" || len(n) > 20 {
			return "", fmt.Errorf("subobject name %q must be 1-20 characters", s.Name)
		}
		out = append(out, LogSubobject{Name: n, Description: s.Description})
	}
	b, err := json.Marshal(map[string]any{"subobjects": out})
	return string(b), err
}

// CreateApplicationLogObject creates an application log object with its subobjects.
func (c *Client) CreateApplicationLogObject(ctx context.Context, o ApplicationLogObjectOptions) (string, error) {
	js, err := buildApplicationLogJSON(o.Subobjects)
	if err != nil {
		return "", fmt.Errorf("CreateApplicationLogObject: %w", err)
	}
	return c.CreateServerDrivenObject(ctx, ServerDrivenOptions{Type: "APLO", Name: o.Name, Description: o.Description,
		Package: o.Package, Transport: o.Transport, JSON: js})
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
