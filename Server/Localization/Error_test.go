package Localization

import (
	"errors"
	"fmt"
	"testing"
)

func TestDescribedErrorsPreserveClassificationAndSpecificContext(t *testing.T) {
	sentinel := errors.New("exact original error")
	inner := WithError(sentinel, New("required", "A positive amount is required.", nil))
	outerRaw := fmt.Errorf("configuration field: %w", inner)
	outer := WithError(outerRaw, ErrorContext(New("field", "Configuration field {name}", Params{"name": "Castle {x}"}), inner))
	if !errors.Is(outer, sentinel) || outer.Error() != outerRaw.Error() {
		t.Fatal("error evidence changed")
	}
	message := FromError(outer)
	if message == nil || message.Key != "required" || len(message.Context) != 1 || message.Context[0].Key != "field" {
		t.Fatalf("context lost: %+v", message)
	}
	if message.FallbackText != outerRaw.Error() {
		t.Fatal("complete raw fallback missing")
	}
	if FromError(fmt.Errorf("unknown outer: %w", inner)) != nil {
		t.Fatal("unknown outer context silently dropped")
	}
	if ErrorContext(New("label", "Label", nil), sentinel) != nil {
		t.Fatal("unknown specific reason masked")
	}
}
