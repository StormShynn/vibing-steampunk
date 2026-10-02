package adt

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// --- FIS: lock object (ENQU) --------------------------------------------------
//
// ADT document read from HL8 (EMEKKOE, 2026-10-02):
//
//	<enqu:lockobject …><enqu:content><enqu:allowRFC>false</enqu:allowRFC>
//	  <enqu:primaryTable><enqu:tableName>EKKO</enqu:tableName><enqu:lockMode>E</enqu:lockMode></enqu:primaryTable>
//	  <enqu:secondaryTables>…</enqu:secondaryTables>
//	  <enqu:lockParameters><enqu:lockParameter><enqu:parameterWanted>true</enqu:parameterWanted>
//	    <enqu:parameterName>EBELN</enqu:parameterName><enqu:tableName>EKKO</enqu:tableName>
//	    <enqu:fieldName>EBELN</enqu:fieldName></enqu:lockParameter>…</enqu:lockParameters>
//	</enqu:content></enqu:lockobject>
//
// The lock modules ENQUEUE_/DEQUEUE_<name> are generated on activation. In ABAP
// Cloud the lock object is used through CL_ABAP_LOCK_OBJECT_FACTORY.

// LockObjectOptions describes a lock object on one table.
type LockObjectOptions struct {
	Name, Description, Package, Transport string
	Table                                 string   // primary table (Z table of the project)
	Fields                                []string // lock parameters = key fields of the table, in key order (include the client field)
	LockMode                              string   // E (default), S, X, O
	AllowRFC                              bool
}

var lockObjectName = regexp.MustCompile(`^E[ZY][A-Z0-9_]*$`)
var lockField = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func buildLockObjectContent(o LockObjectOptions) (string, error) {
	table := strings.ToUpper(strings.TrimSpace(o.Table))
	if !fisXMLName.MatchString(table) {
		return "", fmt.Errorf("table %q must be a Z/Y table", o.Table)
	}
	mode := strings.ToUpper(strings.TrimSpace(o.LockMode))
	if mode == "" {
		mode = "E"
	}
	switch mode {
	case "E", "S", "X", "O":
	default:
		return "", fmt.Errorf("lock_mode %q: use E (write), S (read), X (exclusive, not cumulative) or O (optimistic)", o.LockMode)
	}
	if len(o.Fields) == 0 {
		return "", fmt.Errorf("fields is required: the key fields of %s in key order, including the client field", table)
	}
	var params strings.Builder
	for _, f := range o.Fields {
		f = strings.ToUpper(strings.TrimSpace(f))
		if !lockField.MatchString(f) {
			return "", fmt.Errorf("field %q is not a valid field name", f)
		}
		fmt.Fprintf(&params, `
      <enqu:lockParameter>
        <enqu:parameterWanted>true</enqu:parameterWanted>
        <enqu:parameterName>%s</enqu:parameterName>
        <enqu:tableName>%s</enqu:tableName>
        <enqu:fieldName>%s</enqu:fieldName>
      </enqu:lockParameter>`, f, table, f)
	}
	return fmt.Sprintf(`  <enqu:content>
    <enqu:allowRFC>%t</enqu:allowRFC>
    <enqu:primaryTable>
      <enqu:tableName>%s</enqu:tableName>
      <enqu:lockMode>%s</enqu:lockMode>
    </enqu:primaryTable>
    <enqu:secondaryTables/>
    <enqu:lockParameters>%s
    </enqu:lockParameters>
  </enqu:content>`, o.AllowRFC, escapeXML(table), mode, params.String()), nil
}

// CreateLockObject creates and activates a lock object (ENQU) on one table.
func (c *Client) CreateLockObject(ctx context.Context, o LockObjectOptions) (string, error) {
	name := strings.ToUpper(strings.TrimSpace(o.Name))
	if !lockObjectName.MatchString(name) || len(name) > 16 {
		return "", fmt.Errorf("CreateLockObject: name %q must start with EZ or EY, A-Z 0-9 _ only, max 16", o.Name)
	}
	content, err := buildLockObjectContent(o)
	if err != nil {
		return "", fmt.Errorf("CreateLockObject: %w", err)
	}
	return c.createAndActivateXML(ctx, "CreateLockObject", ObjectTypeLockObject, name, o.Description, o.Package, o.Transport, content)
}
