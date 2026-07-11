package GameCommands

import "testing"

func TestMarketPayloads(t *testing.T) {
	if got, want := CMIPayload(), `%xt%EmpireEx_21%cmi%1%{"S":1,"KID":-1}%`; got != want {
		t.Fatalf("CMIPayload() = %q, want %q", got, want)
	}
	if got, want := CRMPayload(0, 1001, 501, 777, "G", 12345), `%xt%EmpireEx_21%crm%1%{"KID":0,"SID":1001,"TX":501,"TY":777,"HBW":-1,"G":[["G",12345]],"PTT":0,"SD":0}%`; got != want {
		t.Fatalf("CRMPayload() = %q, want %q", got, want)
	}
}

func TestCraftingSkipPayload(t *testing.T) {
	if got, want := CRSKPayload(0, 1001, 77, 1, "production", 75), `%xt%EmpireEx_21%crsk%1%{"KID":0,"AID":1001,"OID":77,"S":1,"ST":"production","PC2":75}%`; got != want {
		t.Fatalf("CRSKPayload() = %q, want %q", got, want)
	}
}
