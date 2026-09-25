package GameData

import "testing"

func TestResponseCodeProducerDescriptorsFollowOverridesAndStayIsolated(t *testing.T) {
	for _, tc := range []struct {
		opcode string
		code   int
	}{{"ahr", 273}, {"rpc", 374}, {"ere", 226}, {"ere", 227}, {"eqe", 236}, {"gui", 53}, {"cra", 256}, {"adi", 95}} {
		meaning := ResolveResponseCode(nil, tc.opcode, tc.code)
		if meaning.MessageDescriptor != nil && meaning.MessageDescriptor.Fallback != meaning.Message {
			t.Fatalf("message override mismatch: %#v", meaning)
		}
		if meaning.Recovery != "" && (meaning.RecoveryDescriptor == nil || meaning.RecoveryDescriptor.Fallback != meaning.Recovery) {
			t.Fatalf("recovery override mismatch: %#v", meaning)
		}
		if meaning.MessageDescriptor != nil {
			meaning.MessageDescriptor.Fallback = "mutated"
		}
		if meaning.RecoveryDescriptor != nil {
			meaning.RecoveryDescriptor.Fallback = "mutated"
		}
		again := ResolveResponseCode(nil, tc.opcode, tc.code)
		if again.MessageDescriptor != nil && again.MessageDescriptor.Fallback == "mutated" || again.RecoveryDescriptor != nil && again.RecoveryDescriptor.Fallback == "mutated" {
			t.Fatal("caller mutated finite source registry")
		}
	}
	language, err := DecodeLanguage([]byte(`{"errorCode_53":"Official context explanation."}`), LanguageMetadata{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	meaning := ResolveResponseCode(language, "gui", 53)
	if meaning.Source != ResponseCodeOfficial || meaning.Message != "Official context explanation." || meaning.MessageDescriptor == nil || meaning.MessageDescriptor.OfficialKey != "errorCode_53" || meaning.RecoveryDescriptor == nil || meaning.RecoveryDescriptor.Fallback != meaning.Recovery {
		t.Fatalf("official precedence or focused recovery lost: %#v", meaning)
	}
}
