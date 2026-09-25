package Intent

import (
	"CitadelDesktop/Server/GameData"
	"testing"
)

func TestIncompleteOfficialExplanationsDoNotInventMissingArguments(t *testing.T) {
	for _, code := range []int{100, 140, 197, 307, 310} {
		meaning := GameData.ResponseCodeMeaning{Code: code, Source: GameData.ResponseCodeOfficial, Message: "Published explanation requires {0}."}
		p := (&Engine{}).failurePresentation(Receipt{}, &ResponseCodeError{Opcode: "test", Meaning: meaning})
		if p.ExplanationDescriptor == nil || p.ExplanationDescriptor.Key != "server.intent.game_declined.incomplete_explanation" || p.ExplanationDescriptor.OfficialKey != "" || len(p.ExplanationDescriptor.Params) != 0 || p.ExplanationDescriptor.Fallback != p.Explanation {
			t.Fatalf("code %d invented or lost evidence: %#v", code, p)
		}
	}
}

func TestObservedResponseExplanationAndGuidanceDescriptorsReachFailure(t *testing.T) {
	meaning := GameData.ResolveResponseCode(nil, "rpc", 374)
	p := (&Engine{}).failurePresentation(Receipt{}, &ResponseCodeError{Opcode: "rpc", Meaning: meaning})
	if p.ExplanationDescriptor == nil || p.ExplanationDescriptor.Fallback != p.Explanation || p.RecoveryDescriptor == nil || p.RecoveryDescriptor.Fallback != p.Recovery {
		t.Fatalf("source-owned response text lost descriptors: %#v", p)
	}
	unknown := (&Engine{}).failurePresentation(Receipt{}, &ResponseCodeError{Meaning: GameData.ResolveResponseCode(nil, "test", 99999)})
	if unknown.ExplanationDescriptor == nil || unknown.ExplanationDescriptor.Key != "server.intent.game_declined.unknown_explanation" || unknown.RecoveryDescriptor == nil || unknown.GameCode == nil || *unknown.GameCode != 99999 {
		t.Fatal("unknown code evidence lost")
	}
}
