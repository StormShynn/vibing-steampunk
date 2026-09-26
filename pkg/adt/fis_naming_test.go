package adt

import (
	"errors"
	"testing"
)

func TestCheckNamingRule(t *testing.T) {
	ok := [][2]string{
		{"CLAS/OC", "ZCL_AP_PAYDOC_SEND"}, {"CLAS/OC", "ZBP_R_PAYDOCTP"}, {"CLAS/OC", "ZCX_AP_ERROR"},
		{"TABL/DT", "ZTB_AP_PAYDOC"}, {"TABL/DS", "ZST_ADMIN_DATA"}, {"DDLS/DF", "ZR_PAYDOCTP"},
		{"DDLX/EX", "ZC_PAYDOCTP"}, {"DCLS/DL", "ZR_PAYDOCTP"}, {"DTEL/DE", "ZDE_AP_AMOUNT"},
		{"DOMA/DD", "ZDO_AP_STATUS"}, {"MSAG/N", "ZMS_AP"}, {"SRVB/SVB", "ZUI_AP_PAYDOC_O4"},
		{"SUSO/B", "Z_AP_PAY"}, {"NROB/NRO", "ZNR_PAYDOC"}, {"APLO/TYP", "ZAL_AP"}, {"SAJC", "ZAJC_AP_SEND"},
		{"SAJT", "ZAJT_AP_SEND"}, {"ENHO/XHB", "ZEI_MM_SLOC"}, {"SIA7/AS", "anything"}, {"PROG/P", "ZANY"},
		{"CLAS/OC", "ZCL_FISST_0927"}, {"MSAG/N", "ZMS_FISST0927"}, {"TABL/DT", "ZTB_FISST_JL0927"}, {"CLAS/OC", "/ABC/CL_X"},
	}
	for _, c := range ok {
		if err := checkNamingRule(c[0], c[1]); err != nil {
			t.Errorf("%s %s: unexpected %v", c[0], c[1], err)
		}
	}
	bad := [][2]string{
		{"CLAS/OC", "YCL"}, {"CLAS/OC", "ZTEST_CLASS"}, {"TABL/DT", "ZTB_AP_PAYDOC_LONG1"}, {"TABL/DT", "ZPAYDOC"},
		{"MSAG/N", "ZFI_MSG"}, {"SAJC", "ZJOB_EINV"}, {"NROB/NRO", "ZNR_TOO_LONG"}, {"SRVB/SVB", "ZUI_AP_PAYDOC"},
		{"DTEL/DE", "ZĐE_X"}, {"DDLX/EX", "ZR_PAYDOCTP"}, {"MSAG/N", "ZFISST0927"}, {"TABL/DT", "ZFISST_0927"},
	}
	for _, c := range bad {
		if err := checkNamingRule(c[0], c[1]); err == nil {
			t.Errorf("%s %s: expected refusal", c[0], c[1])
		}
	}
}

func TestNamingPrefix(t *testing.T) {
	for in, want := range map[string]string{"ZJOB_EINV": "ZJOB_", "Z_AP_PAY": "Z_AP_", "ZAP03": "ZAP", "ZFISST_JC": "ZFISST_", "ZAM02_X": "ZAM02_"} {
		if got := namingPrefix(in); got != want {
			t.Errorf("%s: %s want %s", in, got, want)
		}
	}
	var pe *namingPrefixError
	if err := checkNamingRule("SAJC", "ZJOB_EINV"); !errors.As(err, &pe) {
		t.Errorf("prefix miss should be negotiable, got %v", err)
	}
	if err := checkNamingRule("TABL/DT", "ZTB_AP_PAYDOC_LONG1"); errors.As(err, &pe) || err == nil {
		t.Errorf("length must be hard, got %v", err)
	}
}
