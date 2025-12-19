package ReconfigureLoadout

// StatNameToIDs contains mappings from frontend stat name strings to their corresponding
// game stat IDs. A stat name may map to multiple IDs because different equipment sources
// (base equipment, gems, heroes) use different ID ranges for the same logical stat.
//
// The optimizer uses these IDs to identify which stats an equipment piece contributes to
// when scoring loadout combinations.

// CommStatNameToIDs maps commander stat names (as used in frontend) to all corresponding stat IDs.
// Each stat name maps to a slice of IDs covering: fixed stats, random relic stats, gem stats, and hero stats.
var CommStatNameToIDs = map[string][]float64{
	// Base Combat Stats
	"meleeCbtStr": {1, 101, 108, 211, 311, 813, 814}, // Fixed + Relic + NPC Gem + CL Gem + Hero CL
	"rangeCbtStr": {2, 102, 109, 212, 312, 813, 814}, // Fixed + Relic + NPC Gem + CL Gem + Hero CL
	"wallStr":     {3, 103, 110, 209, 309, 808},      // Fixed + Relic + NPC Gem + CL Gem + Hero CL
	"gateStr":     {4, 104, 111, 210, 310, 809},      // Fixed + Relic + NPC Gem + CL Gem + Hero CL
	"moatStr":     {5, 105, 112, 216, 317, 807},      // Fixed + Relic + NPC Gem + CL Gem + Hero CL
	"travel":      {6, 106, 113, 20020},              // Fixed + Relic + Hero Bonus
	"loot":        {7, 107, 114},                     // Fixed + Relic

	// Relic 2.0 Stats (random only)
	"flankLimit":  {115},
	"cyCbtStr":    {116, 214, 314, 810}, // Relic + NPC Gem + CL Gem + Hero CL
	"frontLimit":  {117},
	"allCbtStr":   {118},
	"frontCbtStr": {119, 215, 315, 812}, // Relic + NPC Gem + CL Gem + Hero CL
	"flankCbtStr": {120, 213, 313, 811}, // Relic + NPC Gem + CL Gem + Hero CL
	"maidenSupp":  {121},

	// Hero Bonus Stats
	"eliteStr":    {20012},
	"horrorStr":   {20013},
	"beserkerStr": {20015},
	"relicStr":    {20016},
	"meadStr":     {20017},
	"wave":        {20018},
	"cooldown":    {20019},

	// NPC Gem/Hero Stats (mapped via special keys from frontend)
	"NPCMelee": {211},
	"NPCRange": {212},
	"NPCFlank": {213},
	"NPCCy":    {214},
	"NPCFront": {215},
	"NPCMoat":  {216},
	"NPCWall":  {209},
	"NPCGate":  {210},
	"NPCGlory": {320},

	// CL Gem/Hero Stats
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

	// Frontend special combined keys (use PvP/NPC context to determine which)
	"glory": {319, 320, 816}, // CLGlory + NPCGlory + Hero CLGlory
	"later": {316, 806},      // CLLater + Hero CLLater
	"fire":  {318, 815},      // CLFire + Hero CLFire
}

// CastStatNameToIDs maps castellan stat names to their corresponding stat IDs.
var CastStatNameToIDs = map[string][]float64{
	// Fixed Stats
	"lootStr":     {10001, 10101, 10216, 10217, 10218, 10219, 10416, 10417},               // Fixed + Random + Gems + Hero
	"wallLimit":   {10002, 10113, 10201, 10301, 10207, 10213, 10307, 10313, 10415, 10815}, // Many sources
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

	// Hero Bonus Stats (Castellan)
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

	// NPC Stats (Castellan)
	"NPCWall":      {10203, 10209, 10409},
	"NPCGate":      {10204, 10210, 10410},
	"NPCMelee":     {10205, 10211, 10411},
	"NPCRange":     {10206, 10212, 10412},
	"NPCWallLimit": {10207, 10213, 10413},
	"NPCCy":        {10208, 10214, 10414},
	"NPCMoat":      {10215, 10415},

	// CL Stats (Castellan)
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

	// Frontend special combined keys (for castellan)
	"glory": {10318, 10818}, // CLGlory + Hero
	"fire":  {10317, 10817}, // CLFire + Hero
	"early": {10315, 10809}, // CLEarly + Hero
}

// PriorityStat represents a stat from the priority list with its tier and position info.
type PriorityStat struct {
	StatName string
	Tier     int
	Position int
	StatIDs  []float64
}

// ConvertPriorityListToStatIDs converts the frontend priority list (string stat names)
// to a structured list with their corresponding game stat IDs.
// It takes the equipment mode ("Commander" or "Castellan") to determine which mapping to use.
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

		// Look up the stat IDs for this stat name
		if ids, exists := statMap[s.Stat]; exists {
			priorityStat.StatIDs = ids
		}

		result = append(result, priorityStat)
	}

	return result
}

// GetStatIDsForName is a helper function that returns all stat IDs for a given stat name
// based on the equipment mode.
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

// IsStatIDInPriority checks if a given stat ID is within the priority list.
// Returns the matching PriorityStat if found, or nil if not.
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
