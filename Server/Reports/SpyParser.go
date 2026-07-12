package Reports

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"CitadelDesktop/Server/State"
)

func ParseSpyCapture(capture State.SpyReportCapture) (SpyReport, error) {
	var wire struct {
		MID int64         `json:"MID"`
		SA  int           `json:"SA"`
		SR  int           `json:"SR"`
		SC  int           `json:"SC"`
		GC  int           `json:"GC"`
		CID int64         `json:"CID"`
		OI  spyPlayerWire `json:"OI"`
		SO  spyPlayerWire `json:"SO"`
		AI  struct {
			Name      string `json:"N"`
			KingdomID int    `json:"K"`
			X         int    `json:"X"`
			Y         int    `json:"Y"`
		} `json:"AI"`
		Setup     [][][]json.RawMessage `json:"S"`
		Castellan json.RawMessage       `json:"B"`
	}
	if err := json.Unmarshal(capture.Payload, &wire); err != nil {
		return SpyReport{}, fmt.Errorf("decode spy report %d: %w", capture.MessageID, err)
	}
	if wire.MID <= 0 {
		wire.MID = capture.MessageID
	}
	report := SpyReport{
		ID: strconv.FormatInt(wire.MID, 10), MID: wire.MID,
		CapturedAtUnixMillis: capture.CapturedAt.UnixMilli(), Status: "failed",
		Accuracy: wire.SA, Risk: wire.SR, SpyCount: wire.SC, GuardCount: wire.GC,
		Target: wire.OI.player(), Source: wire.SO.player(),
		Castle: Castle{ID: wire.CID, Name: wire.AI.Name, KingdomID: wire.AI.KingdomID, X: wire.AI.X, Y: wire.AI.Y},
		Setup:  []SpySection{},
	}
	if capture.CapturedAt.IsZero() {
		report.CapturedAtUnixMillis = time.Now().UnixMilli()
	}
	names := []string{"Left flank", "Center", "Right flank", "Courtyard", "Reserve I", "Reserve II", "Reserve III"}
	for index, rows := range wire.Setup {
		section := SpySection{Index: index, Name: fmt.Sprintf("Section %d", index+1), Units: []UnitCount{}}
		if index < len(names) {
			section.Name = names[index]
		}
		for _, row := range rows {
			if len(row) < 2 {
				continue
			}
			unitID, unitOK := rawInteger(row[0])
			amount, amountOK := rawInteger(row[1])
			if !unitOK || !amountOK || unitID <= 0 || amount <= 0 {
				continue
			}
			section.Units = append(section.Units, UnitCount{WodID: unitID, Amount: amount})
			section.Total += amount
		}
		if len(section.Units) > 0 {
			report.Setup = append(report.Setup, section)
			report.TotalTroops += section.Total
		}
	}
	if len(wire.Castellan) > 0 && string(wire.Castellan) != "null" && string(wire.Castellan) != "{}" {
		var castellan struct {
			Level     int `json:"L"`
			Wall      int `json:"W"`
			Gate      int `json:"GID"`
			Moat      int `json:"D"`
			Courtyard int `json:"SPR"`
			WallSlots int `json:"ST"`
			Effects   any `json:"AE"`
			Equipment any `json:"EQ"`
			SkillIDs  any `json:"SIDS"`
		}
		if json.Unmarshal(wire.Castellan, &castellan) == nil {
			report.Castellan = &SpyCastellan{
				Level: castellan.Level, Wall: castellan.Wall, Gate: castellan.Gate,
				Moat: castellan.Moat, Courtyard: castellan.Courtyard, WallSlots: castellan.WallSlots,
				Effects: castellan.Effects, Equipment: castellan.Equipment, SkillIDs: castellan.SkillIDs,
			}
		}
	}
	if len(report.Setup) > 0 && report.Castellan != nil {
		report.Status = "success"
	} else if len(report.Setup) > 0 || report.Castellan != nil {
		report.Status = "partial"
	}
	return report, nil
}

type spyPlayerWire struct {
	ID       int64  `json:"OID"`
	Name     string `json:"N"`
	Alliance string `json:"AN"`
}

func (wire spyPlayerWire) player() Player {
	return Player{ID: wire.ID, Name: wire.Name, Alliance: wire.Alliance}
}

func rawInteger(raw json.RawMessage) (int64, bool) {
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		value, err := number.Int64()
		return value, err == nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		value, err := strconv.ParseInt(text, 10, 64)
		return value, err == nil
	}
	return 0, false
}
