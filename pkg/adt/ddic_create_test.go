package adt

import (
	"strings"
	"testing"
)

// Document HL8 (S/4HANA Cloud Public Edition) returned for a domain right
// after the minimal POST, 2026-09-26: no adtcore:description, empty datatype.
const hl8NewDomainDoc = `<?xml version="1.0" encoding="utf-8"?><doma:domain adtcore:responsible="CB9980000003" adtcore:masterLanguage="EN" adtcore:name="ZDO_FISST_0926" adtcore:type="DOMA/DD" adtcore:version="new" adtcore:language="EN" xmlns:doma="http://www.sap.com/dictionary/domain" xmlns:adtcore="http://www.sap.com/adt/core"><adtcore:packageRef adtcore:uri="/sap/bc/adt/packages/ytest_nghiabht" adtcore:type="DEVC/K" adtcore:name="YTEST_NGHIABHT" adtcore:description="Test"/><doma:content><doma:typeInformation><doma:datatype/><doma:length>000000</doma:length><doma:decimals>000000</doma:decimals></doma:typeInformation><doma:outputInformation><doma:length>000000</doma:length><doma:style>00</doma:style><doma:conversionExit/><doma:signExists>false</doma:signExists><doma:lowercase>false</doma:lowercase><doma:ampmFormat>false</doma:ampmFormat></doma:outputInformation><doma:valueInformation><doma:valueTableRef/><doma:appendExists>false</doma:appendExists><doma:fixValues/></doma:valueInformation></doma:content></doma:domain>`

func TestSetRootDescriptionAddsMissingAttribute(t *testing.T) {
	desc := `Trạng thái "A&B"`
	got, err := setRootDescription(hl8NewDomainDoc, "doma:domain", desc)
	if err != nil {
		t.Fatal(err)
	}
	root := got[strings.Index(got, "<doma:domain"):]
	root = root[:strings.IndexByte(root, '>')]
	if !strings.Contains(root, `adtcore:description="`+escapeXML(desc)+`"`) || strings.Contains(root, `A&B`) {
		t.Fatalf("root has no description: %s", root)
	}
	if !strings.Contains(got, `adtcore:name="YTEST_NGHIABHT" adtcore:description="Test"/>`) {
		t.Fatal("packageRef description must stay untouched")
	}
}

func TestSetRootDescriptionReplacesExisting(t *testing.T) {
	doc := `<blue:wbobj adtcore:name="Z" adtcore:description="old" xmlns:blue="x"><a/></blue:wbobj>`
	got, _ := setRootDescription(doc, "blue:wbobj", "new")
	if strings.Count(got, "adtcore:description") != 1 || !strings.Contains(got, `adtcore:description="new"`) {
		t.Fatalf("got %s", got)
	}
}

func TestEditDomainDocTypeOutputAndFixedValues(t *testing.T) {
	o := DomainOptions{DataType: "CHAR", Length: 2, FixedValues: []DomainFixedValue{
		{Low: "01", Text: "Mở"}, {Low: "02", Text: "Đóng"}, {Low: "10", High: "19", Text: "Khoảng"},
	}}
	got, err := editDomainDoc(hl8NewDomainDoc, o)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<doma:datatype>CHAR</doma:datatype><doma:length>000002</doma:length><doma:decimals>000000</doma:decimals></doma:typeInformation>",
		"<doma:outputInformation><doma:length>000002</doma:length>",
		"<doma:fixValue><doma:position>0001</doma:position><doma:low>01</doma:low><doma:high></doma:high><doma:text>Mở</doma:text></doma:fixValue>",
		"<doma:position>0003</doma:position><doma:low>10</doma:low><doma:high>19</doma:high>",
		"</doma:fixValues></doma:valueInformation>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestEditDomainDocDecimalsOutputLength(t *testing.T) {
	got, _ := editDomainDoc(hl8NewDomainDoc, DomainOptions{DataType: "DEC", Length: 13, Decimals: 2, Lowercase: true})
	if !strings.Contains(got, "<doma:outputInformation><doma:length>000014</doma:length>") ||
		!strings.Contains(got, "<doma:lowercase>true</doma:lowercase>") {
		t.Fatalf("got %s", got)
	}
}

func TestValidateFixedValues(t *testing.T) {
	cases := []struct {
		o  DomainOptions
		ok bool
	}{
		{DomainOptions{Length: 2, FixedValues: []DomainFixedValue{{Low: "01", Text: "a"}}}, true},
		{DomainOptions{Length: 2, FixedValues: []DomainFixedValue{{Low: "001", Text: "a"}}}, false},
		{DomainOptions{Length: 2, FixedValues: []DomainFixedValue{{Low: "ab", Text: "a"}}}, false},
		{DomainOptions{Length: 2, Lowercase: true, FixedValues: []DomainFixedValue{{Low: "ab", Text: "a"}}}, true},
		{DomainOptions{Length: 2, FixedValues: []DomainFixedValue{{Low: "01"}}}, false},
		{DomainOptions{Length: 2, FixedValues: []DomainFixedValue{{Low: "01", Text: "a"}, {Low: "01", Text: "b"}}}, false},
	}
	for i, c := range cases {
		if err := validateFixedValues(c.o); (err == nil) != c.ok {
			t.Errorf("case %d: err=%v want ok=%v", i, err, c.ok)
		}
	}
}
