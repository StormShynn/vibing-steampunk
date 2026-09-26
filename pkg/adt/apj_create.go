package adt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// --- Application job catalog entry (SAJC) and job template (SAJT) ---------
//
// Both are "server-driven" (blue) objects on S/4HANA Cloud Public Edition:
// the object document is a blue:blueSource shell and the content is a JSON
// source at <object>/source/main, e.g. for a catalog entry
//
//	{"formatVersion":"1",
//	 "header":{"description":"…","originalLanguage":"en","abapLanguageVersion":"cloudDevelopment"},
//	 "generalInformation":{"className":"ZCL_…"}}
//
// (read from HL8 with GetADTObjectXML, 2026-09-26). Creating them through
// ADT is what the Eclipse wizard does; the ABAP API CL_APJ_DT_CREATE_CONTENT
// cannot be used for this from ABAP Cloud (the template call is refused by
// SAP's released-object check).
//
// Flow: POST shell -> LOCK -> PUT JSON source -> UNLOCK -> ACTIVATE.

const (
	apjCatalogCollection  = "/sap/bc/adt/applicationjob/catalogs"
	apjTemplateCollection = "/sap/bc/adt/applicationjob/templates"
	bluesV2ContentType    = "application/vnd.sap.adt.blues.v2+xml"
)

var (
	apjObjectName = regexp.MustCompile(`^[ZY][A-Z0-9_]{0,29}$`)
	apjClassName  = regexp.MustCompile(`^(/[A-Z0-9_]{1,10}/)?[A-Z0-9_]{1,30}$`)
)

// JobCatalogOptions describes an application job catalog entry to create.
type JobCatalogOptions struct {
	Name        string
	Description string
	ClassName   string // class implementing IF_APJ_DT_EXEC_OBJECT + IF_APJ_RT_EXEC_OBJECT
	Package     string
	Transport   string
}

// JobTemplateOptions describes an application job template to create.
type JobTemplateOptions struct {
	Name        string
	Description string
	CatalogName string // existing job catalog entry
	Package     string
	Transport   string
}

type apjHeader struct {
	Description         string `json:"description"`
	OriginalLanguage    string `json:"originalLanguage"`
	AbapLanguageVersion string `json:"abapLanguageVersion"`
}

type apjCatalogSource struct {
	FormatVersion      string    `json:"formatVersion"`
	Header             apjHeader `json:"header"`
	GeneralInformation struct {
		ClassName string `json:"className"`
	} `json:"generalInformation"`
}

type apjTemplateSource struct {
	FormatVersion      string    `json:"formatVersion"`
	Header             apjHeader `json:"header"`
	GeneralInformation struct {
		CatalogName string `json:"catalogName"`
	} `json:"generalInformation"`
}

func (c *Client) apjLanguage() string {
	lang := strings.ToLower(strings.TrimSpace(c.Language()))
	if len(lang) != 2 {
		return "en"
	}
	return lang
}

func buildCatalogSource(description, className, lang string) ([]byte, error) {
	var s apjCatalogSource
	s.FormatVersion = "1"
	s.Header = apjHeader{Description: description, OriginalLanguage: lang, AbapLanguageVersion: "cloudDevelopment"}
	s.GeneralInformation.ClassName = className
	return json.MarshalIndent(s, "", "  ")
}

func buildTemplateSource(description, catalogName, lang string) ([]byte, error) {
	var s apjTemplateSource
	s.FormatVersion = "1"
	s.Header = apjHeader{Description: description, OriginalLanguage: lang, AbapLanguageVersion: "cloudDevelopment"}
	s.GeneralInformation.CatalogName = catalogName
	return json.MarshalIndent(s, "", "  ")
}

func buildBlueShell(name, adtType, description, pkg string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<blue:blueSource xmlns:blue="http://www.sap.com/wbobj/blue" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="%s" adtcore:type="%s" adtcore:description="%s">
  <adtcore:packageRef adtcore:name="%s"/>
</blue:blueSource>`, name, adtType, escapeXML(description), pkg)
}

func validateAPJ(opName, name, description, ref, refLabel string) (string, string, error) {
	name = strings.ToUpper(strings.TrimSpace(name))
	ref = strings.ToUpper(strings.TrimSpace(ref))
	if !apjObjectName.MatchString(name) {
		return "", "", fmt.Errorf("%s: name %q must start with Z or Y, A-Z 0-9 _ only, max 30", opName, name)
	}
	if strings.TrimSpace(description) == "" {
		return "", "", fmt.Errorf("%s: description is required", opName)
	}
	if len([]rune(description)) > 60 {
		return "", "", fmt.Errorf("%s: description longer than 60 characters", opName)
	}
	if !apjClassName.MatchString(ref) {
		return "", "", fmt.Errorf("%s: %s %q is not a valid name", opName, refLabel, ref)
	}
	return name, ref, nil
}

// CreateJobCatalogEntry creates and activates an application job catalog entry (SAJC).
func (c *Client) CreateJobCatalogEntry(ctx context.Context, o JobCatalogOptions) (string, error) {
	name, class, err := validateAPJ("CreateJobCatalogEntry", o.Name, o.Description, o.ClassName, "class_name")
	if err != nil {
		return "", err
	}
	src, err := buildCatalogSource(o.Description, class, c.apjLanguage())
	if err != nil {
		return "", err
	}
	return c.blueJSONCreate(ctx, "CreateJobCatalogEntry", apjCatalogCollection, "SAJC", name, o.Description, o.Package, o.Transport, src)
}

// CreateJobTemplate creates and activates an application job template (SAJT).
func (c *Client) CreateJobTemplate(ctx context.Context, o JobTemplateOptions) (string, error) {
	name, catalog, err := validateAPJ("CreateJobTemplate", o.Name, o.Description, o.CatalogName, "catalog_name")
	if err != nil {
		return "", err
	}
	src, err := buildTemplateSource(o.Description, catalog, c.apjLanguage())
	if err != nil {
		return "", err
	}
	return c.blueJSONCreate(ctx, "CreateJobTemplate", apjTemplateCollection, "SAJT", name, o.Description, o.Package, o.Transport, src)
}

// blueJSONCreate: POST blue shell -> LOCK -> PUT JSON source -> UNLOCK -> ACTIVATE.
func (c *Client) blueJSONCreate(ctx context.Context, opName, collection, adtType, name, description, pkg, transport string, source []byte) (string, error) {
	if pkg == "" {
		pkg = "$TMP"
	}
	pkg = strings.ToUpper(pkg)
	if err := c.checkMutation(ctx, MutationContext{Op: OpCreate, OpName: opName, Package: pkg, Transport: transport}); err != nil {
		return "", err
	}

	params := url.Values{}
	if transport != "" {
		params.Set("corrNr", transport)
	}
	if _, err := c.transport.Request(ctx, collection, &RequestOptions{
		Method: http.MethodPost, Query: params, Body: []byte(buildBlueShell(name, adtType, description, pkg)),
		ContentType: bluesV2ContentType, Accept: bluesV2ContentType,
	}); err != nil {
		return "", fmt.Errorf("%s: creating %s: %w", opName, name, err)
	}

	objURL := collection + "/" + url.PathEscape(strings.ToLower(name))
	ctx = withMutationPackageChecked(ctx, objURL)

	lock, err := c.LockObject(ctx, objURL, "MODIFY")
	if err != nil {
		return objURL, fmt.Errorf("%s: %s was created but could not be locked to write its content: %w", opName, name, err)
	}
	put := url.Values{}
	put.Set("lockHandle", lock.LockHandle)
	if transport != "" {
		put.Set("corrNr", transport)
	}
	if _, err := c.transport.Request(ctx, objURL+"/source/main", &RequestOptions{
		Method: http.MethodPut, Query: put, Body: source,
		ContentType: "application/json; charset=utf-8", Accept: "application/json", Stateful: true,
	}); err != nil {
		if uerr := c.releaseLockAfterFailure(ctx, objURL, lock.LockHandle); uerr != nil {
			return objURL, fmt.Errorf("%s: writing content: %w — %s", opName, err, strandedLockAdvice(objURL, uerr))
		}
		return objURL, fmt.Errorf("%s: writing content: %w (object %s exists, inactive — fix it in ADT or delete it)", opName, err, name)
	}
	if err := c.UnlockObject(ctx, objURL, lock.LockHandle); err != nil {
		return objURL, fmt.Errorf("%s: unlocking %s: %w — %s", opName, name, err, strandedLockAdvice(objURL, err))
	}
	activation, err := c.Activate(ctx, objURL, name)
	if err == nil && !activation.Success {
		// Seen on HL8 (2026-09-26) for both SAJC and SAJT: the first activation
		// right after the content PUT reports the referenced class / catalog as
		// empty ("Report or class  is invalid", "Job catalog entry  doesn't
		// exist"), a second activation of the same object succeeds.
		activation, err = c.Activate(ctx, objURL, name)
	}
	if err != nil {
		return objURL, fmt.Errorf("%s: activating %s: %w", opName, name, err)
	}
	if !activation.Success {
		return objURL, fmt.Errorf("%s: %s was created but did not activate: %s", opName, name, strings.Join(activation.ProblemLines(), "; "))
	}
	return objURL, nil
}
