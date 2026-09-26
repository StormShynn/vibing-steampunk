package adt

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// --- Naming rule on object creation (always on, no setting) ----------------
//
// A new object must carry a recognisable type prefix, so a reader knows what it
// is from the name alone (not ZFISST_JC, ZFISST0926 …). Accepted, in order:
//
//  1. the FIS / SAP-recommended prefix for the type (table below), or
//  2. the convention the tenant already uses for that type: some existing
//     object of the same type starts with the same prefix (the part up to and
//     including the first "_" after the namespace letter, else the leading
//     letters — ZJOB_…, ZAM…). Projects that follow their own convention keep
//     working; the first object of a new pattern needs the FIS prefix.
//
// Hard in every case: A-Z 0-9 _ only, and the length limit of the type.
// Updates, deletes and activation are not checked. Namespaced names (/ABC/…)
// are left to the namespace owner. The prefix table is the error level of the
// plugin's scripts/naming_lint.py.

type namingRule struct {
	max  int
	re   *regexp.Regexp
	hint string
}

func nr(max int, pattern, hint string) namingRule {
	return namingRule{max, regexp.MustCompile(pattern), hint}
}

// fisNamingRules is keyed by the main ADT type (the part before "/").
var fisNamingRules = map[string]namingRule{
	"TABL": nr(16, `^[ZY]TB_`, "table ZTB_<MOD>_<entity> (draft ZTB_…_D)"),
	"STRU": nr(30, `^[ZY]ST_`, "structure ZST_<MOD>_<name>"),
	"DDLS": nr(30, `^[ZY](R|C|I|A)_`, "CDS ZR_ / ZC_ / ZI_ / ZA_"),
	"BDEF": nr(30, `^[ZY](R|C)_`, "behavior definition = view name ZR_ / ZC_"),
	"DDLX": nr(30, `^[ZY]C_`, "metadata extension = projection name ZC_"),
	"DCLS": nr(30, `^[ZY](R|C|I)_`, "access control = protected view name"),
	"CLAS": nr(30, `^[ZY](CL|BP_R|CX)_`, "class ZCL_ / behavior pool ZBP_R_ / exception ZCX_"),
	"INTF": nr(30, `^[ZY]IF_`, "interface ZIF_"),
	"DTEL": nr(30, `^[ZY]DE_`, "data element ZDE_"),
	"DOMA": nr(30, `^[ZY]DO_`, "domain ZDO_"),
	"TTYP": nr(30, `^[ZY]TT_`, "table type ZTT_"),
	"MSAG": nr(20, `^[ZY]MS_`, "message class ZMS_<MOD>"),
	"SRVD": nr(30, `^[ZY](UI|API)_`, "service definition ZUI_ / ZAPI_"),
	"SRVB": nr(30, `^[ZY](UI|API)_.*_O[24]$`, "service binding ZUI_…_O4 / ZAPI_…_O4 (_O2)"),
	"SUSO": nr(10, `^[ZY]_`, "authorization object Z_<MOD>_<obj>"),
	"AUTH": nr(10, `^[ZY]`, "authorization field Z…"),
	"NROB": nr(10, `^[ZY]NR_`, "number range ZNR_<…>"),
	"ENHO": nr(30, `^[ZY]EI_`, "BAdI implementation ZEI_<MOD>_<…>"),
	"SIA6": nr(30, `^[ZY]IAM_`, "IAM app ZIAM_<MOD>_<…>"),
	"SIA1": nr(30, `^[ZY]BC_`, "business catalog ZBC_<MOD>_<…>"),
	"SCO1": nr(30, `^[ZY]CS_`, "communication scenario ZCS_<…>"),
	"APLO": nr(20, `^[ZY]AL_`, "application log object ZAL_<MOD>"),
	"SAJC": nr(30, `^[ZY]AJC_`, "job catalog entry ZAJC_<MOD>_<…>"),
	"SAJT": nr(30, `^[ZY]AJT_`, "job template ZAJT_<MOD>_<…>"),
	"DEVC": nr(30, `^[ZY]`, "package Z… (ZPK_<PROJECT>_<MOD> or the project's package tree)"),
}

var namingASCII = regexp.MustCompile(`^[A-Z/][A-Z0-9_/]*$`)

// namingKey maps an ADT type ("CLAS/OC", "TABL/DS", "SUSO/B") to a rule key.
func namingKey(adtType string) string {
	t := strings.ToUpper(adtType)
	if t == "TABL/DS" {
		return "STRU"
	}
	if i := strings.IndexByte(t, '/'); i > 0 {
		return t[:i]
	}
	return t
}

// namingPrefix returns the pattern a name shares with its siblings:
// "ZJOB_EINV" -> "ZJOB_", "Z_AP_PAY" -> "Z_AP_", "ZAP03" -> "ZAP".
func namingPrefix(n string) string {
	if k := strings.IndexByte(n[1:], '_'); k >= 0 {
		p := n[:k+2]
		if p == "Z_" || p == "Y_" { // authorization objects Z_<MOD>_…
			if k2 := strings.IndexByte(n[2:], '_'); k2 >= 0 {
				return n[:k2+3]
			}
		}
		return p
	}
	i := 1
	for i < len(n) && n[i] >= 'A' && n[i] <= 'Z' {
		i++
	}
	return n[:i]
}

// checkFISNaming refuses to create an object whose name has neither the FIS
// prefix for its type nor a prefix the tenant already uses for that type.
func (c *Client) checkFISNaming(ctx context.Context, adtType, name string) error {
	err := checkNamingRule(adtType, name)
	if err == nil {
		return nil
	}
	var pe *namingPrefixError
	if !errors.As(err, &pe) {
		return err // characters / length: never negotiable
	}
	prefix := namingPrefix(pe.name)
	if len(prefix) < 3 {
		return err
	}
	hits, serr := c.SearchObject(ctx, prefix+"*", 50)
	if serr != nil {
		return fmt.Errorf("%w (could not check the tenant's existing names: %v)", err, serr)
	}
	for _, h := range hits {
		if strings.EqualFold(h.Name, pe.name) {
			continue
		}
		if namingKey(h.Type) == pe.key && strings.HasPrefix(strings.ToUpper(h.Name), prefix) {
			return nil // follows a convention already used on the tenant for this type
		}
	}
	return fmt.Errorf("%w; no existing %s on the tenant starts with %s either — use the FIS prefix, or the prefix the project already uses for %s", err, pe.key, prefix, pe.key)
}

// namingPrefixError: the name is valid but lacks the FIS prefix for its type.
type namingPrefixError struct {
	key, name, hint string
}

func (e *namingPrefixError) Error() string {
	return fmt.Sprintf("naming: %s %s has no recognisable type prefix (FIS: %s)", e.key, e.name, e.hint)
}

func checkNamingRule(adtType, name string) error {
	n := strings.ToUpper(strings.TrimSpace(name))
	if n == "" {
		return nil
	}
	key := namingKey(adtType)
	rule, ok := fisNamingRules[key]
	if !ok {
		return nil
	}
	if strings.HasPrefix(n, "/") {
		return nil // namespaced (/ABC/…) — the namespace owner's convention applies
	}
	if !namingASCII.MatchString(n) {
		return fmt.Errorf("naming: %s %s — only A-Z 0-9 _ (no Vietnamese accents)", key, name)
	}
	if len(n) > rule.max {
		return fmt.Errorf("naming: %s %s is %d characters, max %d", key, n, len(n), rule.max)
	}
	if !rule.re.MatchString(n) {
		return &namingPrefixError{key: key, name: n, hint: rule.hint}
	}
	return nil
}
