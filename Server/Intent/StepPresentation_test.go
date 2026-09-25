package Intent

import (
	"CitadelDesktop/Server/Localization"
	"reflect"
	"testing"
)

func TestStepNameDescriptorPreservesCommandAndSource(t *testing.T) {
	original := Step{Name: "Renew Castle {admin}", Opcode: "buy", Payload: []byte(`{"id":4}`), AwaitOpcode: "buy", SuccessCodes: []int{0, 1}, FinalDispatchAction: "verify-purchase"}
	source := Localization.New("test.renew", "Renew {name}", Localization.Params{"name": "Castle {admin}"})
	result := original.WithNameDescriptor(source)
	if result.NameDescriptor == source || result.NameDescriptor.FallbackText != original.Name || result.NameDescriptor.Fallback != "Renew {name}" {
		t.Fatal("descriptor was not independently bound to verbatim legacy name")
	}
	result.NameDescriptor.Params["name"] = "changed"
	if source.Params["name"] != "Castle {admin}" || source.FallbackText != "" {
		t.Fatal("source descriptor mutated")
	}
	result.NameDescriptor = nil
	if !reflect.DeepEqual(result, original) {
		t.Fatal("presentation decorator changed command fields")
	}
	if original.WithNameDescriptor(nil).NameDescriptor != nil {
		t.Fatal("missing descriptor must remain absent")
	}
}
