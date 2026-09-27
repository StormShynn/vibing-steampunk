package adt

import (
	"strings"
	"testing"
)

func TestEditMessageClassDoc(t *testing.T) {
	doc := `<mc:messageClass adtcore:name="ZMS"><mc:messages adtcore:name="" mc:documented="false" mc:msgno="001" mc:msgtext="old" mc:selfexplainatory="true"><atom:link href="a"/></mc:messages></mc:messageClass>`
	out, err := editMessageClassDoc(doc, []MessageClassMessage{{Number: "001", Text: "new & <1>"}, {Number: "002", Text: "two &1"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`mc:msgno="001" mc:msgtext="new &amp; &lt;1&gt;"`, `<atom:link href="a"/>`, `mc:msgno="002" mc:msgtext="two &amp;1" mc:selfexplainatory="true"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
	if _, err := editMessageClassDoc(doc, []MessageClassMessage{{Number: "1", Text: "x"}}); err == nil {
		t.Error("want error for bad number")
	}
}
