package GameParser

import (
	"reflect"
	"testing"

	"CitadelDesktop/Server/Models"
)

func TestAllianceShareRecipientsUsesFullAINRoster(t *testing.T) {
	members := []Models.AllianceMember{
		{PlayerID: 10},
		{PlayerID: 20},
		{PlayerID: 30},
		{PlayerID: 20},
		{PlayerID: 0},
		{PlayerID: 40},
	}
	got := allianceShareRecipients(members, 30)
	want := []int{10, 20, 40}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("allianceShareRecipients() = %v, want %v", got, want)
	}
}
