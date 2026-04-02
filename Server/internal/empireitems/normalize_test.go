package empireitems

import "testing"

func TestRefineDecoParticleOf(t *testing.T) {
	if got := RefineDecoParticleOf("Gardenof Meditation"); got != "Garden Of Meditation" {
		t.Fatalf("got %q", got)
	}
	if got := RefineDecoParticleOf("Garden Of Meditation"); got != "Garden Of Meditation" {
		t.Fatalf("idempotent: got %q", got)
	}
}

func TestFormatDisplayNameFromInternalType(t *testing.T) {
	if got := FormatDisplayNameFromInternalType("Supplies1"); got != "Supplies 1" {
		t.Fatalf("got %q", got)
	}
	if got := FormatDisplayNameFromInternalType("VictoryMemorial"); got != "Victory Memorial" {
		t.Fatalf("got %q", got)
	}
}
