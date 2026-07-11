package GameCommands

import (
	"context"
	"encoding/json"
	"fmt"

	"CitadelDesktop/Server/Automation"
)

// Fixed slot counts per flank in EmpireEx **cra** (verified vs Logs/RecvCommandsJSON/cra_launch.json).
const (
	CRALRFlankToolSlots       = 2
	CRALRFlankUnitSlots       = 2
	CRAMFlankToolSlots        = 3
	CRAMFlankUnitSlots        = 6
	CRASupportTroopSlots      = 8
	CRAAttackSupportToolSlots = 3

	// CRAHBWCoinMovementBooster is the coin movement booster on **cra** when not using a travel feather.
	// The client also supports 1008 and 1009; this builder only models feather vs coin (1007).
	CRAHBWCoinMovementBooster = 1007
)

// CRATroopPair is one [wodID, amount] row on the wire; empty slots use [-1, 0].
type CRATroopPair [2]int

// CRAFlank is one attack flank: **T** = tools, **U** = units.
type CRAFlank struct {
	T []CRATroopPair `json:"T"`
	U []CRATroopPair `json:"U"`
}

// CRAWave is one attack wave with left, right, and middle flanks.
type CRAWave struct {
	L CRAFlank `json:"L"`
	R CRAFlank `json:"R"`
	M CRAFlank `json:"M"`
}

// CRALaunchBody is the JSON body for outbound **cra** (castle attack launch).
// Inbound **cra** responses use a different shape (AAM); see Logs/RecvCommandsJSON/cra.json.
type CRALaunchBody struct {
	SX   int            `json:"SX"`
	SY   int            `json:"SY"`
	TX   int            `json:"TX"`
	TY   int            `json:"TY"`
	KID  int            `json:"KID"`
	LID  int            `json:"LID"` // commanderID on the wire
	WT   int            `json:"WT"`
	HBW  int            `json:"HBW"`
	BPC  int            `json:"BPC"`
	ATT  int            `json:"ATT"`
	AV   int            `json:"AV"` // 0 = route/preview shell, 1 = launch; if **cra** fails, retry the other value
	LP   int            `json:"LP"`
	FC   int            `json:"FC"`
	PTT  int            `json:"PTT"`
	SD   int            `json:"SD"`
	ICA  int            `json:"ICA"`
	CD   int            `json:"CD"`
	A    []CRAWave      `json:"A"`
	BKS  []any          `json:"BKS"`
	AST  []int          `json:"AST"` // attack support tools: tool wodID per slot (-1 = empty)
	RW   []CRATroopPair `json:"RW"`  // support troops: [unit wodID, count] per slot ([-1,0] = empty)
	ASCT int            `json:"ASCT"`
}

// CRALaunchParams configures an outbound **cra** frame.
type CRALaunchParams struct {
	SourceX, SourceY int
	TargetX, TargetY int
	KingdomID        int
	CommanderID      int // wire **LID** — attacking commander id (UM.L.ID / general slot)
	WaitHours        int // WT
	// UseTravelFeather selects the movement-booster pair on the wire (**HBW** / **PTT**):
	//   true  → travel feather: HBW -1, PTT 1
	//   false → coin booster:   HBW 1007, PTT 0
	UseTravelFeather bool
	// AttackValid is wire **AV**: 0 = route/preview shell, 1 = launch with troops.
	// If **cra** is rejected, retry once with the other AV value (0 ↔ 1).
	AttackValid        int
	Waves              []CRAWave
	AttackSupportTools []int          // wire **AST**: attack support tool wodID per slot (3; pad -1)
	SupportTroops      []CRATroopPair // wire **RW**: support troop per slot as [unit wodID, count] (8; pad [-1,0])
}

// craHBWPTT maps UseTravelFeather to the **cra** movement-booster fields.
func craHBWPTT(useTravelFeather bool) (hbw, ptt int) {
	if useTravelFeather {
		return -1, 1
	}
	return CRAHBWCoinMovementBooster, 0
}

// CRAPair returns a troop/tool pair for **cra** flank arrays.
func CRAPair(wodID, amount int) CRATroopPair {
	return CRATroopPair{wodID, amount}
}

// CRAEmptyPair is the wire empty slot [-1, 0].
func CRAEmptyPair() CRATroopPair {
	return CRATroopPair{-1, 0}
}

func craPadPairs(pairs []CRATroopPair, size int) []CRATroopPair {
	out := make([]CRATroopPair, size)
	for i := range out {
		out[i] = CRAEmptyPair()
	}
	for i, p := range pairs {
		if i >= size {
			break
		}
		out[i] = p
	}
	return out
}

func craPadAttackSupportTools(toolIDs []int) []int {
	out := make([]int, CRAAttackSupportToolSlots)
	for i := range out {
		out[i] = -1
	}
	for i, id := range toolIDs {
		if i >= CRAAttackSupportToolSlots {
			break
		}
		out[i] = id
	}
	return out
}

// CRAFlankLR builds a left/right flank with fixed tool/unit slot counts.
func CRAFlankLR(tools, units []CRATroopPair) CRAFlank {
	return CRAFlank{
		T: craPadPairs(tools, CRALRFlankToolSlots),
		U: craPadPairs(units, CRALRFlankUnitSlots),
	}
}

// CRAFlankM builds the middle flank (3 tool slots, 6 unit slots).
func CRAFlankM(tools, units []CRATroopPair) CRAFlank {
	return CRAFlank{
		T: craPadPairs(tools, CRAMFlankToolSlots),
		U: craPadPairs(units, CRAMFlankUnitSlots),
	}
}

// CRAWaveFromFlanks assembles one wave from the three flanks.
func CRAWaveFromFlanks(left, right, middle CRAFlank) CRAWave {
	return CRAWave{L: left, R: right, M: middle}
}

// CRAEmptyFlankLR is an all-empty left/right flank.
func CRAEmptyFlankLR() CRAFlank {
	return CRAFlankLR(nil, nil)
}

// CRAEmptyFlankM is an all-empty middle flank.
func CRAEmptyFlankM() CRAFlank {
	return CRAFlankM(nil, nil)
}

// CRAEmptyWave is one wave with every slot empty.
func CRAEmptyWave() CRAWave {
	return CRAWaveFromFlanks(CRAEmptyFlankLR(), CRAEmptyFlankLR(), CRAEmptyFlankM())
}

// CRADummyMaidenProbeUnitWodID and count match Logs/RecvCommandsJSON/cra_launch.json outbound_empty_shell:
// one wave, unit 216 × 11 on left, right, and middle flanks (minimal shield probe).
const (
	CRADummyMaidenProbeUnitWodID = 216
	CRADummyMaidenProbeUnitCount = 11
)

// CRADummyMaidenProbeWave builds the minimal 1-wave maiden-comms probe layout.
// unitWodID/unitCount default to CRADummyMaidenProbeUnitWodID / CRADummyMaidenProbeUnitCount when <= 0.
func CRADummyMaidenProbeWave(unitWodID, unitCount int) CRAWave {
	if unitWodID <= 0 {
		unitWodID = CRADummyMaidenProbeUnitWodID
	}
	if unitCount <= 0 {
		unitCount = CRADummyMaidenProbeUnitCount
	}
	pair := CRAPair(unitWodID, unitCount)
	return CRAWaveFromFlanks(
		CRAFlankLR(nil, []CRATroopPair{pair}),
		CRAFlankLR(nil, []CRATroopPair{pair}),
		CRAFlankM(nil, []CRATroopPair{pair}),
	)
}

// BuildCRALaunchBody builds the JSON object for **cra** from params.
func BuildCRALaunchBody(p CRALaunchParams) CRALaunchBody {
	hbw, ptt := craHBWPTT(p.UseTravelFeather)

	waves := p.Waves
	if len(waves) == 0 {
		waves = []CRAWave{CRAEmptyWave()}
	}

	return CRALaunchBody{
		SX:   p.SourceX,
		SY:   p.SourceY,
		TX:   p.TargetX,
		TY:   p.TargetY,
		KID:  p.KingdomID,
		LID:  p.CommanderID,
		WT:   p.WaitHours,
		HBW:  hbw,
		BPC:  0,
		ATT:  0,
		AV:   p.AttackValid,
		LP:   0,
		FC:   0,
		PTT:  ptt,
		SD:   0,
		ICA:  0,
		CD:   99,
		A:    waves,
		BKS:  []any{},
		AST:  craPadAttackSupportTools(p.AttackSupportTools),
		RW:   craPadPairs(p.SupportTroops, CRASupportTroopSlots),
		ASCT: 0,
	}
}

// CRALaunchBodyJSON returns compact JSON for the **cra** body.
func CRALaunchBodyJSON(p CRALaunchParams) (string, error) {
	b, err := json.Marshal(BuildCRALaunchBody(p))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CRAPayload returns EmpireEx_21 **cra** — launch a castle attack with wave/flank troop layout.
//
// **LID** is commander id. **HBW**/**PTT** from UseTravelFeather. **AST** = attack support tool ids;
// **RW** = support troops as [unit wodID, count] pairs (same slot idea as AST, with quantity).
// **AV** (AttackValid): 0 preview shell vs 1 full launch — if the server rejects **cra**, retry with the other AV.
// Live shape: Logs/RecvCommandsJSON/cra_launch.json (outbound). Inbound AAM response: cra.json.
func CRAPayload(p CRALaunchParams) (string, error) {
	body, err := CRALaunchBodyJSON(p)
	if err != nil {
		return "", err
	}
	return empireExFrame("cra", body), nil
}

// CRAPayloadFromBody wraps a parsed or stored **cra** body in an EmpireEx frame.
func CRAPayloadFromBody(body CRALaunchBody) (string, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	return empireExFrame("cra", string(b)), nil
}

// SendCRA queues **cra** on the attack-launch lane.
// On failure, callers may retry the same params with AttackValid toggled (0 ↔ 1).
// Before dispatch, check gamestate.GetGameState().IsCommanderWireIDBusy(CommanderID) so busy commanders are not reused.
func SendCRA(p CRALaunchParams) error {
	payload, err := CRAPayload(p)
	if err != nil {
		return err
	}
	receipt := DispatchPayload(context.Background(), "cra", "manual_attack", payload, Automation.CommandOptions{
		Owner:    Automation.OwnerManual,
		Priority: Automation.PriorityManual,
	})
	if !receipt.Accepted {
		return fmt.Errorf("command harness rejected cra: %s", receipt.Message)
	}
	return nil
}
