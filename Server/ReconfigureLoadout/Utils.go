package ReconfigureLoadout

// CommStatNameToIDs maps commander stat names to all corresponding stat IDs.
var CommStatNameToIDs = map[string][]float64{
	// Base Stats
	"meleeCbtStr": {1, 101, 108, 211, 311, 813, 814},
	"rangeCbtStr": {2, 102, 109, 212, 312, 813, 814},
	"wallStr":     {3, 103, 110, 209, 309, 808},
	"gateStr":     {4, 104, 111, 210, 310, 809},
	"moatStr":     {5, 105, 112, 216, 317, 807},
	"travel":      {6, 106, 113, 20020},
	"loot":        {7, 107, 114},

	// Relic 2.0
	"flankLimit":  {115},
	"cyCbtStr":    {116, 214, 314, 810},
	"frontLimit":  {117},
	"allCbtStr":   {118},
	"frontCbtStr": {119, 215, 315, 812},
	"flankCbtStr": {120, 213, 313, 811},
	"maidenSupp":  {121},

	// Hero Bonus
	"eliteStr":    {20012},
	"horrorStr":   {20013},
	"beserkerStr": {20015},
	"relicStr":    {20016},
	"meadStr":     {20017},
	"wave":        {20018},
	"cooldown":    {20019},

	// NPC Stats
	"NPCMelee": {211},
	"NPCRange": {212},
	"NPCFlank": {213},
	"NPCCy":    {214},
	"NPCFront": {215},
	"NPCMoat":  {216},
	"NPCWall":  {209},
	"NPCGate":  {210},
	"NPCGlory": {320},

	// CL Stats
	"CLWall":  {309, 808},
	"CLGate":  {310, 809},
	"CLMelee": {311, 813},
	"CLRange": {312, 814},
	"CLFlank": {313, 811},
	"CLCy":    {314, 810},
	"CLFront": {315, 812},
	"CLLater": {316, 806},
	"CLMoat":  {317, 807},
	"CLFire":  {318, 815},
	"CLGlory": {319, 816},

	// Combined
	"glory": {319, 320, 816},
	"later": {316, 806},
	"fire":  {318, 815},
}

// CastStatNameToIDs maps castellan stat names to their corresponding stat IDs.
var CastStatNameToIDs = map[string][]float64{
	// Fixed Stats
	"lootStr":     {10001, 10101, 10216, 10217, 10218, 10219, 10416, 10417},
	"wallLimit":   {10002, 10113, 10201, 10301, 10207, 10213, 10307, 10313, 10415, 10815},
	"gateStr":     {10003, 10103, 10109, 10204, 10210, 10304, 10310, 10410, 10812},
	"moatStr":     {10004, 10110, 10215, 10316, 10415, 10810},
	"meleeCbtStr": {10005, 10111, 10205, 10211, 10305, 10311, 10411, 10813},
	"rangeCbtStr": {10006, 10112, 10206, 10212, 10306, 10312, 10412, 10814},

	// Random Relic Stats
	"wallStr":       {10102, 10108},
	"cyCbtStr":      {10114, 10202, 10208, 10302, 10308, 10414, 10816},
	"allCbtStr":     {10115},
	"frontCbtStr":   {10116},
	"flankCbtStr":   {10117},
	"protectorSupp": {10118},

	// Hero Bonus
	"research":     {30001},
	"recruit":      {30009},
	"hospital":     {30010},
	"construction": {30011},
	"baseRes":      {30012},
	"kingRes":      {30013},
	"po":           {30014},
	"resTransport": {30016},
	"meadProd":     {30017},
	"honeyProd":    {30018},
	"meadStorage":  {30019},
	"honeyStorage": {30020},

	// Hero Stats
	"mainCbtStr": {10807},
	"opCbtStr":   {10808},

	// NPC Stats
	"NPCWall":      {10203, 10209, 10409},
	"NPCGate":      {10204, 10210, 10410},
	"NPCMelee":     {10205, 10211, 10411},
	"NPCRange":     {10206, 10212, 10412},
	"NPCWallLimit": {10207, 10213, 10413},
	"NPCCy":        {10208, 10214, 10414},
	"NPCMoat":      {10215, 10415},

	// CL Stats
	"CLWall":      {10303, 10309, 10811},
	"CLGate":      {10304, 10310, 10812},
	"CLMelee":     {10305, 10311, 10813},
	"CLRange":     {10306, 10312, 10814},
	"CLWallLimit": {10307, 10313, 10815},
	"CLCy":        {10308, 10314, 10816},
	"CLEarly":     {10315, 10809},
	"CLMoat":      {10316, 10810},
	"CLFire":      {10317, 10817},
	"CLGlory":     {10318, 10818},

	// Combined
	"glory": {10318, 10818},
	"fire":  {10317, 10817},
	"early": {10315, 10809},
}

// PriorityStat definition...
type PriorityStat struct {
	StatName string
	Tier     int
	Position int
	StatIDs  []float64
}

// ConvertPriorityListToStatIDs converts...
func ConvertPriorityListToStatIDs(stats []struct {
	Stat     string `json:"stat"`
	Tier     int    `json:"tier"`
	Position int    `json:"position"`
}, equipmentMode string) []PriorityStat {
	result := make([]PriorityStat, 0, len(stats))

	// Select the appropriate mapping based on equipment mode
	var statMap map[string][]float64
	if equipmentMode == "Castellan" {
		statMap = CastStatNameToIDs
	} else {
		statMap = CommStatNameToIDs
	}

	for _, s := range stats {
		priorityStat := PriorityStat{
			StatName: s.Stat,
			Tier:     s.Tier,
			Position: s.Position,
			StatIDs:  []float64{},
		}

		if ids, exists := statMap[s.Stat]; exists {
			priorityStat.StatIDs = ids
		}

		result = append(result, priorityStat)
	}

	return result
}

// GetStatIDsForName helper
func GetStatIDsForName(statName string, equipmentMode string) []float64 {
	var statMap map[string][]float64
	if equipmentMode == "Castellan" {
		statMap = CastStatNameToIDs
	} else {
		statMap = CommStatNameToIDs
	}

	if ids, exists := statMap[statName]; exists {
		return ids
	}
	return []float64{}
}

// IsStatIDInPriority...
func IsStatIDInPriority(statID float64, priorityStats []PriorityStat) *PriorityStat {
	for i := range priorityStats {
		for _, id := range priorityStats[i].StatIDs {
			if id == statID {
				return &priorityStats[i]
			}
		}
	}
	return nil
}
