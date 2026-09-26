package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// --- DDIC create: data elements and domains -------------------------------
//
// Both follow the pattern CreateTable already uses on S/4HANA Cloud Public
// Edition: POST a minimal object, then LOCK -> GET the server's own document
// -> substitute the values -> PUT it back under the same vocabulary type ->
// UNLOCK -> ACTIVATE. Editing the document the server hands out, instead of
// sending a hand-built one, means every element and flag this code does not
// know about keeps its server default rather than being dropped.

const (
	dtelCollection  = "/sap/bc/adt/ddic/dataelements"
	dtelContentType = "application/vnd.sap.adt.dataelements.v2+xml"
	domaCollection  = "/sap/bc/adt/ddic/domains"
	domaContentType = "application/vnd.sap.adt.domains.v2+xml"
)

// DataElementOptions describes a data element to create.
type DataElementOptions struct {
	Name        string
	Description string
	Package     string
	Transport   string
	// Either Domain (typeKind=domain) or DataType+Length(+Decimals)
	// (typeKind=predefinedAbapType) must be set.
	Domain   string
	DataType string // CHAR, NUMC, DEC, INT4, DATS, TIMS, CURR, QUAN, CUKY, UNIT, STRG, ...
	Length   int
	Decimals int
	// Field labels (short <=10, medium <=20, long <=40, heading <=55).
	Short, Medium, Long, Heading string
	// ChangeDocument sets the "change document" flag of the element.
	ChangeDocument bool
}

// DomainOptions describes a domain to create.
type DomainOptions struct {
	Name        string
	Description string
	Package     string
	Transport   string
	DataType    string
	Length      int
	Decimals    int
	OutputLen   int  // 0 = same as Length
	Lowercase   bool // allow lower-case values
	// FixedValues replaces the domain's fixed values (single values or
	// intervals). Empty = leave the server default (none).
	FixedValues []DomainFixedValue
}

// DomainFixedValue is one fixed value (Low) or interval (Low..High) of a domain.
type DomainFixedValue struct {
	Low  string
	High string
	Text string // short text, max 60
}

// ddicCreate runs POST (minimal) -> LOCK -> GET -> edit -> PUT -> UNLOCK -> ACTIVATE.
func (c *Client) ddicCreate(ctx context.Context, opName, collection, contentType, adtType, rootTag, rootNS,
	name, description, pkg, transport string, edit func(doc string) (string, error)) (string, error) {

	name = strings.ToUpper(name)
	if name == "" || len(name) > 30 {
		return "", fmt.Errorf("%s: name must be 1-30 characters", opName)
	}
	if pkg == "" {
		pkg = "$TMP"
	}
	if err := c.checkMutation(ctx, MutationContext{Op: OpCreate, OpName: opName, Package: pkg, Transport: transport}); err != nil {
		return "", err
	}
	if err := c.checkFISNaming(ctx, adtType, name); err != nil {
		return "", err
	}
	if err := c.requireTransportFor(ctx, collection+"/"+url.PathEscape(strings.ToLower(name)), pkg, transport); err != nil {
		return "", fmt.Errorf("%s: %w", opName, err)
	}

	create := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<%[1]s xmlns:%[2]s xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="%[3]s" adtcore:type="%[4]s" adtcore:description="%[5]s">
  <adtcore:packageRef adtcore:name="%[6]s"/>
</%[1]s>`, rootTag, rootNS, name, adtType, escapeXML(description), pkg)

	params := url.Values{}
	if transport != "" {
		params.Set("corrNr", transport)
	}
	if _, err := c.transport.Request(ctx, collection, &RequestOptions{
		Method: http.MethodPost, Query: params, Body: []byte(create),
		ContentType: contentType, Accept: contentType,
	}); err != nil {
		return "", fmt.Errorf("%s: creating %s: %w", opName, name, err)
	}

	objURL := collection + "/" + url.PathEscape(strings.ToLower(name))
	// Package was approved above for the create; do not resolve it again from
	// inside the lock window (issue #91).
	ctx = withMutationPackageChecked(ctx, objURL)

	lock, err := c.LockObject(ctx, objURL, "MODIFY")
	if err != nil {
		return objURL, fmt.Errorf("%s: %s was created but could not be locked to set its attributes: %w", opName, name, err)
	}
	fail := func(step string, e error) (string, error) {
		if uerr := c.releaseLockAfterFailure(ctx, objURL, lock.LockHandle); uerr != nil {
			return objURL, fmt.Errorf("%s: %s: %w — %s", opName, step, e, strandedLockAdvice(objURL, uerr))
		}
		return objURL, fmt.Errorf("%s: %s: %w (object %s exists, inactive — fix it in ADT or delete it)", opName, step, e, name)
	}

	resp, err := c.transport.Request(ctx, objURL, &RequestOptions{
		Method: http.MethodGet, Accept: contentType, Stateful: true,
	})
	if err != nil {
		return fail("reading the created object", err)
	}
	doc, err := edit(string(resp.Body))
	if err != nil {
		return fail("preparing the object document", err)
	}
	// The document the server returns right after the POST carries no
	// adtcore:description (seen on S/4HANA Cloud Public Edition, 2026-09-26),
	// and the PUT is then refused with SWB_TOOL 019 "The description is
	// missing". Always write the description back onto the root element.
	if doc, err = setRootDescription(doc, rootTag, description); err != nil {
		return fail("preparing the object document", err)
	}

	put := url.Values{}
	put.Set("lockHandle", lock.LockHandle)
	if transport != "" {
		put.Set("corrNr", transport)
	}
	if _, err := c.transport.Request(ctx, objURL, &RequestOptions{
		Method: http.MethodPut, Query: put, Body: []byte(doc),
		ContentType: contentType, Accept: contentType, Stateful: true,
	}); err != nil {
		return fail("writing the object document", err)
	}
	if err := c.UnlockObject(ctx, objURL, lock.LockHandle); err != nil {
		return objURL, fmt.Errorf("%s: unlocking %s: %w — %s", opName, name, err, strandedLockAdvice(objURL, err))
	}

	activation, err := c.Activate(ctx, objURL, name)
	if err != nil {
		return objURL, fmt.Errorf("%s: activating %s: %w", opName, name, err)
	}
	if !activation.Success {
		return objURL, fmt.Errorf("%s: %s was created but did not activate: %s", opName, name, strings.Join(activation.ProblemLines(), "; "))
	}
	return objURL, nil
}

// CreateDataElement creates and activates a data element.
func (c *Client) CreateDataElement(ctx context.Context, o DataElementOptions) (string, error) {
	o.Domain = strings.ToUpper(strings.TrimSpace(o.Domain))
	o.DataType = strings.ToUpper(strings.TrimSpace(o.DataType))
	if o.Domain == "" && (o.DataType == "" || o.Length <= 0 && !fixedLengthType(o.DataType)) {
		return "", fmt.Errorf("CreateDataElement: set domain, or data_type with length")
	}
	for _, l := range []struct {
		v   string
		max int
		n   string
	}{{o.Short, 10, "short"}, {o.Medium, 20, "medium"}, {o.Long, 40, "long"}, {o.Heading, 55, "heading"}} {
		if utf8.RuneCountInString(l.v) > l.max {
			return "", fmt.Errorf("CreateDataElement: %s label %q is longer than %d characters", l.n, l.v, l.max)
		}
	}
	return c.ddicCreate(ctx, "CreateDataElement", dtelCollection, dtelContentType, "DTEL/DE",
		"blue:wbobj", `blue="http://www.sap.com/wbobj/dictionary/dtel"`,
		o.Name, o.Description, o.Package, o.Transport,
		func(doc string) (string, error) { return editDataElementDoc(doc, o) })
}

// CreateDomain creates and activates a domain, with optional fixed values.
func (c *Client) CreateDomain(ctx context.Context, o DomainOptions) (string, error) {
	o.DataType = strings.ToUpper(strings.TrimSpace(o.DataType))
	if o.DataType == "" || o.Length <= 0 && !fixedLengthType(o.DataType) {
		return "", fmt.Errorf("CreateDomain: data_type and length are required")
	}
	if err := validateFixedValues(o); err != nil {
		return "", fmt.Errorf("CreateDomain: %w", err)
	}
	return c.ddicCreate(ctx, "CreateDomain", domaCollection, domaContentType, "DOMA/DD",
		"doma:domain", `doma="http://www.sap.com/dictionary/domain"`,
		o.Name, o.Description, o.Package, o.Transport,
		func(doc string) (string, error) { return editDomainDoc(doc, o) })
}

func fixedLengthType(t string) bool {
	switch t {
	case "INT1", "INT2", "INT4", "INT8", "DATS", "TIMS", "STRG", "RSTR", "STRING", "RAWSTRING", "D16N", "D34N", "UTCL", "DATN", "TIMN":
		return true
	}
	return false
}

// setElem replaces the text of <prefix:tag>…</prefix:tag> or an empty
// <prefix:tag/>; it reports whether the element was present.
func setElem(doc, qname, value string) (string, bool) {
	v := escapeXML(value)
	full := regexp.MustCompile(`<` + regexp.QuoteMeta(qname) + `>[^<]*</` + regexp.QuoteMeta(qname) + `>`)
	if loc := full.FindStringIndex(doc); loc != nil {
		return doc[:loc[0]] + "<" + qname + ">" + v + "</" + qname + ">" + doc[loc[1]:], true
	}
	empty := regexp.MustCompile(`<` + regexp.QuoteMeta(qname) + `\s*/>`)
	if loc := empty.FindStringIndex(doc); loc != nil {
		return doc[:loc[0]] + "<" + qname + ">" + v + "</" + qname + ">" + doc[loc[1]:], true
	}
	return doc, false
}

var rootDescAttr = regexp.MustCompile(`\sadtcore:description="[^"]*"`)

// setRootDescription sets adtcore:description on the document's root element,
// adding the attribute when the server left it out.
func setRootDescription(doc, rootTag, description string) (string, error) {
	i := strings.Index(doc, "<"+rootTag)
	if i < 0 {
		return doc, fmt.Errorf("root element %s not found in the server document", rootTag)
	}
	j := strings.IndexByte(doc[i:], '>')
	if j < 0 {
		return doc, fmt.Errorf("root element %s is not closed", rootTag)
	}
	j += i
	head, tail := doc[i:j], ""
	if strings.HasSuffix(head, "/") {
		head, tail = head[:len(head)-1], "/"
	}
	attr := ` adtcore:description="` + escapeXML(description) + `"`
	if rootDescAttr.MatchString(head) {
		head = rootDescAttr.ReplaceAllLiteralString(head, attr)
	} else {
		head += attr
	}
	return doc[:i] + head + tail + doc[j:], nil
}

func editDataElementDoc(doc string, o DataElementOptions) (string, error) {
	var ok bool
	set := func(tag, val string, required bool) error {
		doc, ok = setElem(doc, "dtel:"+tag, val)
		if !ok && required {
			return fmt.Errorf("element dtel:%s not found in the server document", tag)
		}
		return nil
	}
	if o.Domain != "" {
		if err := set("typeKind", "domain", true); err != nil {
			return "", err
		}
		if err := set("typeName", o.Domain, true); err != nil {
			return "", err
		}
	} else {
		if err := set("typeKind", "predefinedAbapType", true); err != nil {
			return "", err
		}
		_ = set("typeName", "", false)
		if err := set("dataType", o.DataType, true); err != nil {
			return "", err
		}
		_ = set("dataTypeLength", fmt.Sprintf("%06d", o.Length), false)
		_ = set("dataTypeDecimals", fmt.Sprintf("%06d", o.Decimals), false)
	}
	for _, l := range []struct{ tag, val string }{
		{"shortField", o.Short}, {"mediumField", o.Medium}, {"longField", o.Long}, {"headingField", o.Heading},
	} {
		if l.val == "" {
			continue
		}
		if err := set(l.tag+"Label", l.val, true); err != nil {
			return "", err
		}
		_ = set(l.tag+"Length", fmt.Sprintf("%02d", utf8.RuneCountInString(l.val)), false)
	}
	if o.ChangeDocument {
		_ = set("changeDocument", "true", false)
	}
	return doc, nil
}

func editDomainDoc(doc string, o DomainOptions) (string, error) {
	out := o.OutputLen
	if out <= 0 {
		out = o.Length
		if o.Decimals > 0 {
			out++ // room for the decimal separator
		}
	}
	// typeInformation comes before outputInformation in the document, so the
	// first doma:length is the type length and the second the output length.
	var ok bool
	if doc, ok = setElem(doc, "doma:datatype", o.DataType); !ok {
		return "", fmt.Errorf("element doma:datatype not found in the server document")
	}
	idx := strings.Index(doc, "<doma:outputInformation")
	typePart, outPart := doc, ""
	if idx > 0 {
		typePart, outPart = doc[:idx], doc[idx:]
	}
	typePart, _ = setElem(typePart, "doma:length", fmt.Sprintf("%06d", o.Length))
	typePart, _ = setElem(typePart, "doma:decimals", fmt.Sprintf("%06d", o.Decimals))
	if outPart != "" {
		outPart, _ = setElem(outPart, "doma:length", fmt.Sprintf("%06d", out))
		if o.Lowercase {
			outPart, _ = setElem(outPart, "doma:lowercase", "true")
		}
	}
	doc = typePart + outPart
	if len(o.FixedValues) > 0 {
		var b strings.Builder
		b.WriteString("<doma:fixValues>")
		for i, fv := range o.FixedValues {
			fmt.Fprintf(&b, "<doma:fixValue><doma:position>%04d</doma:position><doma:low>%s</doma:low><doma:high>%s</doma:high><doma:text>%s</doma:text></doma:fixValue>",
				i+1, escapeXML(fv.Low), escapeXML(fv.High), escapeXML(fv.Text))
		}
		b.WriteString("</doma:fixValues>")
		loc := fixValuesElem.FindStringIndex(doc)
		if loc == nil {
			return "", fmt.Errorf("element doma:fixValues not found in the server document")
		}
		doc = doc[:loc[0]] + b.String() + doc[loc[1]:]
	}
	return doc, nil
}

var fixValuesElem = regexp.MustCompile(`(?s)<doma:fixValues\s*/>|<doma:fixValues>.*?</doma:fixValues>`)

func validateFixedValues(o DomainOptions) error {
	seen := map[string]bool{}
	for i, fv := range o.FixedValues {
		if utf8.RuneCountInString(fv.Text) > 60 {
			return fmt.Errorf("fixed value %d: text longer than 60 characters", i+1)
		}
		if fv.Text == "" {
			return fmt.Errorf("fixed value %d (%q): text is required", i+1, fv.Low)
		}
		for _, v := range []string{fv.Low, fv.High} {
			if o.Length > 0 && utf8.RuneCountInString(v) > o.Length {
				return fmt.Errorf("fixed value %q is longer than the domain length %d", v, o.Length)
			}
			if !o.Lowercase && v != strings.ToUpper(v) {
				return fmt.Errorf("fixed value %q has lower-case letters but lowercase=false", v)
			}
		}
		k := fv.Low + "\x00" + fv.High
		if seen[k] {
			return fmt.Errorf("fixed value %q is listed twice", fv.Low)
		}
		seen[k] = true
	}
	return nil
}

// GetDDICObjectXML returns the raw ADT document of a data element or domain —
// every attribute the element-level readers leave out (flags, lengths,
// fixed values, change-document flag).
func (c *Client) GetDDICObjectXML(ctx context.Context, objectType, name string) (string, error) {
	if err := c.checkSafety(OpRead, "GetDDICObjectXML"); err != nil {
		return "", err
	}
	var coll, ct string
	switch strings.ToUpper(objectType) {
	case "DTEL":
		coll, ct = dtelCollection, dtelContentType
	case "DOMA":
		coll, ct = domaCollection, domaContentType
	default:
		return "", fmt.Errorf("object_type must be DTEL or DOMA")
	}
	resp, err := c.transport.Request(ctx, coll+"/"+url.PathEscape(strings.ToLower(name)), &RequestOptions{
		Method: http.MethodGet, Accept: ct,
	})
	if err != nil {
		return "", err
	}
	return string(resp.Body), nil
}
