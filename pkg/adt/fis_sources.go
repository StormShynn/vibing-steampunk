package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- FIS: more source-based object types for S/4HANA Cloud Public Edition --
//
// Metadata extensions (DDLX), access controls (DCLS) and DDIC structures
// (TABL/DS) are plain-source objects like a CDS view: POST a shell, then PUT
// the source to <object>/source/main and activate. Message classes (MSAG) are
// a shell plus an XML body (texts are written with WriteMessageClassTexts).
//
// URIs and document roots read from HL8 (2026-09-27): DDLX
// /sap/bc/adt/ddic/ddlx/sources (ddlx:ddlxSource, ns ddlxsources), DCLS
// /sap/bc/adt/acm/dcl/sources, structure /sap/bc/adt/ddic/structures
// (blue:blueSource, application/vnd.sap.adt.structures.v2+xml), MSAG
// /sap/bc/adt/messageclass.

const (
	ObjectTypeDDLX         CreatableObjectType = "DDLX/EX" // CDS metadata extension
	ObjectTypeDCLS         CreatableObjectType = "DCLS/DL" // CDS access control
	ObjectTypeStructure    CreatableObjectType = "TABL/DS" // DDIC structure (define structure)
	ObjectTypeMessageClass CreatableObjectType = "MSAG/N"  // message class
)

// fisSourceType maps a WriteSource/GetSource code to its ADT type.
type fisSourceType struct {
	objType    CreatableObjectType
	collection string
	label      string
}

// fisSourceTypes are the extra plain-source types WriteSource/GetSource accept.
var fisSourceTypes = map[string]fisSourceType{
	"DDLX": {ObjectTypeDDLX, "/sap/bc/adt/ddic/ddlx/sources", "metadata extension"},
	"DCLS": {ObjectTypeDCLS, "/sap/bc/adt/acm/dcl/sources", "access control"},
	"STRU": {ObjectTypeStructure, "/sap/bc/adt/ddic/structures", "structure"},
}

func init() {
	objectTypes[ObjectTypeDDLX] = objectTypeInfo{
		creationPath: "/sap/bc/adt/ddic/ddlx/sources",
		rootName:     "ddlx:ddlxSource",
		namespace:    `xmlns:ddlx="http://www.sap.com/adt/ddic/ddlxsources"`,
	}
	objectTypes[ObjectTypeDCLS] = objectTypeInfo{
		creationPath: "/sap/bc/adt/acm/dcl/sources",
		rootName:     "dcl:dclSource",
		namespace:    `xmlns:dcl="http://www.sap.com/adt/acm/dclsources"`,
	}
	objectTypes[ObjectTypeStructure] = objectTypeInfo{
		creationPath: "/sap/bc/adt/ddic/structures",
		rootName:     "blue:blueSource",
		namespace:    `xmlns:blue="http://www.sap.com/wbobj/blue"`,
		contentType:  "application/vnd.sap.adt.structures.v2+xml",
	}
	objectTypes[ObjectTypeMessageClass] = objectTypeInfo{
		creationPath: "/sap/bc/adt/messageclass",
		rootName:     "mc:messageClass",
		namespace:    `xmlns:mc="http://www.sap.com/adt/MessageClass"`,
		contentType:  "application/vnd.sap.adt.mc.messageclass+xml",
	}
}

// fisObjectURL returns the object URL for the FIS object types (empty if not one of them).
func fisObjectURL(objectType CreatableObjectType, name string) string {
	lower := url.PathEscape(strings.ToLower(name))
	switch objectType {
	case ObjectTypeDDLX:
		return "/sap/bc/adt/ddic/ddlx/sources/" + lower
	case ObjectTypeDCLS:
		return "/sap/bc/adt/acm/dcl/sources/" + lower
	case ObjectTypeStructure:
		return "/sap/bc/adt/ddic/structures/" + lower
	case ObjectTypeMessageClass:
		return "/sap/bc/adt/messageclass/" + lower
	}
	return fisXMLObjectURL(objectType, name)
}

// fisSourceTypeFor returns the ADT type for a WriteSource code (DDLX, DCLS, STRU).
func fisSourceTypeFor(code string) (fisSourceType, bool) {
	t, ok := fisSourceTypes[strings.ToUpper(code)]
	return t, ok
}

// getFISSource reads the source of a DDLX / DCLS / structure.
func (c *Client) getFISSource(ctx context.Context, code, name string) (string, error) {
	t, ok := fisSourceTypeFor(code)
	if !ok {
		return "", fmt.Errorf("unsupported object type: %s", code)
	}
	src := fisObjectURL(t.objType, name) + "/source/main"
	resp, err := c.transport.Request(ctx, src, &RequestOptions{Method: http.MethodGet, Accept: "text/plain"})
	if err != nil {
		return "", fmt.Errorf("getting %s source: %w", t.label, err)
	}
	return string(resp.Body), nil
}

// MessageClassOptions describes a message class to create.
type MessageClassOptions struct {
	Name        string
	Description string
	Package     string
	Transport   string
	Messages    []MessageClassMessage // optional: written right after creation
	Language    string                // language of the texts (default: logon language)
}

// CreateMessageClass creates a message class and, when messages are given, writes them.
func (c *Client) CreateMessageClass(ctx context.Context, o MessageClassOptions) (string, error) {
	name := strings.ToUpper(strings.TrimSpace(o.Name))
	if name == "" || len(name) > 20 || !(strings.HasPrefix(name, "Z") || strings.HasPrefix(name, "Y")) {
		return "", fmt.Errorf("CreateMessageClass: name %q must start with Z or Y, max 20 characters", o.Name)
	}
	if strings.TrimSpace(o.Description) == "" {
		return "", fmt.Errorf("CreateMessageClass: description is required")
	}
	for _, m := range o.Messages {
		if len(m.Number) != 3 || strings.Trim(m.Number, "0123456789") != "" {
			return "", fmt.Errorf("CreateMessageClass: message number %q must be 3 digits", m.Number)
		}
		if len([]rune(m.Text)) > 73 {
			return "", fmt.Errorf("CreateMessageClass: message %s longer than 73 characters", m.Number)
		}
	}
	if err := c.CreateObject(ctx, CreateObjectOptions{
		ObjectType: ObjectTypeMessageClass, Name: name, Description: o.Description,
		PackageName: o.Package, Transport: o.Transport,
	}); err != nil {
		return "", fmt.Errorf("CreateMessageClass: %w", err)
	}
	objURL := fisObjectURL(ObjectTypeMessageClass, name)
	if len(o.Messages) == 0 {
		return objURL, nil
	}
	lang := o.Language
	if lang == "" {
		lang = c.Language()
	}
	ctx = withMutationPackageChecked(ctx, objURL)
	lock, err := c.LockObject(ctx, objURL, "MODIFY")
	if err != nil {
		return objURL, fmt.Errorf("CreateMessageClass: %s created, locking to write texts failed: %w", name, err)
	}
	werr := c.WriteMessageClassTexts(ctx, name, lang, o.Messages, lock.LockHandle, o.Transport)
	if uerr := c.UnlockObject(ctx, objURL, lock.LockHandle); uerr != nil && werr == nil {
		return objURL, fmt.Errorf("CreateMessageClass: unlocking %s: %w", name, uerr)
	}
	if werr != nil {
		return objURL, fmt.Errorf("CreateMessageClass: %s created, writing texts failed: %w", name, werr)
	}
	return objURL, nil
}
