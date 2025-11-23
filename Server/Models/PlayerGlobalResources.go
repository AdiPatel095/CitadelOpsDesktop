package Models

import "sync"

type PlayerGlobalResources struct {
	Rubies     float64 `json:"rubies"`
	Coins      float64 `json:"coins"`
	RelicShard float64 `json:"relic_shard"`
	Sceat      float64 `json:"sceat"`
	Ducat      float64 `json:"ducat"`
	ConstToken float64 `json:"const_token"`
	UpgrToken  float64 `json:"upgr_token"`
	AfflTix    float64 `json:"affl_tix"`
	Plaster    float64 `json:"plaster"`
	DrgScale   float64 `json:"drg_scale"`
	DrgSpl     float64 `json:"drg_spl"`
	Min1       float64 `json:"min1"`
	Min5       float64 `json:"min5"`
	Min10      float64 `json:"min10"`
	Min30      float64 `json:"min30"`
	Hr1        float64 `json:"hr1"`
	Hr5        float64 `json:"hr5"`
	Hr24       float64 `json:"hr24"`
	MightPt    float64 `json:"might_pt"`
	GloryPt    float64 `json:"glory_pt"`
	GallanPt   float64 `json:"gallan_pt"`
}

var (
	instance *PlayerGlobalResources
	once     sync.Once
)

// GetPlayerGlobalResources returns the singleton instance of PlayerGlobalResources.
func GetPlayerGlobalResources() *PlayerGlobalResources {
	once.Do(func() {
		instance = &PlayerGlobalResources{}
	})
	return instance
}
