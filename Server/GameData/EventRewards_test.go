package GameData

import "testing"

func TestScalableEventRewardProgressUsesDifficultyCeiling(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"leaguetypeevents":[
			{"leaguetypeEventsID":"1","eventID":"72","leaguetypeID":"5","rewardSetID":"2",
			 "difficultyIDforMaxPoints":"309,310","difficultyMaxPoints":"300,700",
			 "difficultyScalingNeededPointsForRewards":"100,200,300,400,500,600,700"}
		]
	}`), SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	progress := store.ScalableEventRewardProgress(72, 309, 5, 2, 250)
	if progress.Reached != 2 || progress.Total != 3 || progress.NextScore != 300 {
		t.Fatalf("progress = %#v", progress)
	}
}
