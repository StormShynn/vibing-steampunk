package adt

// FIS fix-8t: create BDEF extensions.
//
// ADT tells an extension from a root BDEF by the ADT template on the create
// request: property base_bdef names the extended BDEF, interface_bdef the BO
// interface of "extension using interface". Without them the BDEF is created
// as a root definition and activation fails with
// "abstract | interface | managed | projection | unmanaged was expected, not extension".
// Keys read from IF_BDEF_ADT_RESSOURCES / CL_BDEF_OBJECT_PERSIST on HL8 (03.10.2026).

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	reBdefLeadingComments = regexp.MustCompile(`^(\s*(//[^\n]*|/\*(?s:.*?)\*/))*\s*`)
	reBdefExtension       = regexp.MustCompile(`(?i)^extension\b`)
	reBdefUsingInterface  = regexp.MustCompile(`(?i)^extension\s+using\s+interface\s+([A-Za-z0-9_/]+)`)
	reBdefName            = regexp.MustCompile(`^[A-Za-z0-9_/]{1,30}$`)
)

// bdefExtensionTargets returns the base BDEF and interface BDEF for a BDEF
// create. Both are empty for an ordinary BDEF. An extension source without
// extends is refused: SAP would otherwise create a root BDEF.
func bdefExtensionTargets(source, extends string) (base, iface string, err error) {
	src := reBdefLeadingComments.ReplaceAllString(source, "")
	isExt := reBdefExtension.MatchString(src)
	extends = strings.ToUpper(strings.TrimSpace(extends))
	if !isExt {
		if extends != "" {
			return "", "", fmt.Errorf("extends is only for a BDEF extension: the source must start with 'extension'")
		}
		return "", "", nil
	}
	if extends == "" {
		return "", "", fmt.Errorf("BDEF extension: pass extends=<extended BDEF> (e.g. R_SALESORDERTP), otherwise SAP creates a root BDEF and activation fails")
	}
	if !reBdefName.MatchString(extends) {
		return "", "", fmt.Errorf("extends %q is not a BDEF name", extends)
	}
	if m := reBdefUsingInterface.FindStringSubmatch(src); m != nil {
		iface = strings.ToUpper(m[1])
	}
	return extends, iface, nil
}

// bdefExtensionTemplate is the adtcore:adtTemplate block for the create body.
func bdefExtensionTemplate(base, iface string) string {
	if base == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n  <adtcore:adtTemplate>")
	fmt.Fprintf(&b, "\n    <adtcore:adtProperty adtcore:key=\"base_bdef\">%s</adtcore:adtProperty>", escapeXML(base))
	if iface != "" {
		fmt.Fprintf(&b, "\n    <adtcore:adtProperty adtcore:key=\"interface_bdef\">%s</adtcore:adtProperty>", escapeXML(iface))
	}
	b.WriteString("\n  </adtcore:adtTemplate>")
	return b.String()
}
