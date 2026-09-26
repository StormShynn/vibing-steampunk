package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// --- FIS extras for S/4HANA Cloud Public Edition --------------------------
//
// GetADTObjectXML: read-only GET of an ADT object document for object types
// vsp has no dedicated reader for (application job catalog entries /
// templates, application log objects, change document objects, IAM objects).
// Used to learn the document format before adding a writer, and to verify
// objects a user created by hand in ADT.
//
// RunClass: runs a class that implements IF_OO_ADT_CLASSRUN — the same
// request ADT sends on F9 — and returns the console text. Executing code is
// treated as a write-class operation: blocked in read-only mode, and the
// class must sit in an allowed package.

// adtXMLReadPrefixes lists the ADT paths GetADTObjectXML may read.
var adtXMLReadPrefixes = []string{
	"/sap/bc/adt/applicationjob/",
	"/sap/bc/adt/applicationlog/",
	"/sap/bc/adt/changedocuments/",
	"/sap/bc/adt/aps/cloud/iam/",
	"/sap/bc/adt/ddic/",
	"/sap/bc/adt/numberranges/",
	"/sap/bc/adt/aps/iam/",       // authorization objects / fields (SUSO, AUTH)
	"/sap/bc/adt/aps/cloud/com/", // communication scenarios (SCO1, SCO2)
	"/sap/bc/adt/acm/",           // access controls (DCLS)
	"/sap/bc/adt/messageclass/",  // message classes
	"/sap/bc/adt/enhancements/",  // BAdI implementations (ENHO)
	"/sap/bc/adt/businessobjects/",
	"/sap/bc/adt/businessservices/",
}

// serverDrivenAccept returns the media type ADT wants for the server-driven
// sub-resources of a blue object (schema / configuration), else "".
func serverDrivenAccept(p string) string {
	switch {
	case strings.HasSuffix(p, "/schema"):
		return "application/vnd.sap.adt.serverdriven.schema.v1+json; framework=objectTypes.v1"
	case strings.HasSuffix(p, "/configuration"):
		return "application/vnd.sap.adt.serverdriven.configuration.v1+json; framework=objectTypes.v1"
	}
	return ""
}

var classRunName = regexp.MustCompile(`^(/[A-Z0-9_]{1,10}/)?[A-Z0-9_]{1,30}$`)

// normalizeADTReadPath validates an ADT object path for GetADTObjectXML.
func normalizeADTReadPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("object_uri is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("object_uri is not a valid URI: %w", err)
	}
	if u.Scheme != "" || u.Host != "" {
		return "", fmt.Errorf("object_uri must be a path on this system (start with /sap/bc/adt/), not a full URL")
	}
	p := u.Path
	if strings.Contains(p, "..") || strings.Contains(p, "//") {
		return "", fmt.Errorf("object_uri must not contain '..' or '//'")
	}
	p = strings.TrimSuffix(p, "/")
	for _, pre := range adtXMLReadPrefixes {
		if strings.HasPrefix(strings.ToLower(p)+"/", pre) && len(p) > len(pre)-1 {
			return p, nil
		}
	}
	return "", fmt.Errorf("object_uri %q is outside the readable ADT areas (%s)", p, strings.Join(adtXMLReadPrefixes, ", "))
}

// GetADTObjectXML returns the raw ADT document of an object under one of the
// readable ADT areas, plus the response content type.
func (c *Client) GetADTObjectXML(ctx context.Context, objectURI string) (string, string, error) {
	if err := c.checkSafety(OpRead, "GetADTObjectXML"); err != nil {
		return "", "", err
	}
	p, err := normalizeADTReadPath(objectURI)
	if err != nil {
		return "", "", err
	}
	accept := "application/*, text/*"
	if sd := serverDrivenAccept(p); sd != "" {
		accept = sd
	}
	resp, err := c.transport.Request(ctx, p, &RequestOptions{
		Method: http.MethodGet,
		Accept: accept,
	})
	if err != nil {
		return "", "", err
	}
	return string(resp.Body), resp.Headers.Get("Content-Type"), nil
}

// RunClass runs an IF_OO_ADT_CLASSRUN class and returns its console output.
func (c *Client) RunClass(ctx context.Context, className string) (string, error) {
	name := strings.ToUpper(strings.TrimSpace(className))
	if !classRunName.MatchString(name) {
		return "", fmt.Errorf("RunClass: %q is not a valid class name", className)
	}
	if err := c.checkSafety(OpWorkflow, "RunClass"); err != nil {
		return "", err
	}
	classURL := "/sap/bc/adt/oo/classes/" + url.PathEscape(strings.ToLower(name))
	if err := c.checkObjectPackageSafety(ctx, classURL); err != nil {
		return "", fmt.Errorf("RunClass: %w", err)
	}
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/oo/classrun/"+url.PathEscape(name), &RequestOptions{
		Method: http.MethodPost,
		Accept: "text/plain",
	})
	if err != nil {
		return "", fmt.Errorf("RunClass %s: %w", name, err)
	}
	return string(resp.Body), nil
}
