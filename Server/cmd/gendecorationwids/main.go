// Command gendecorationwids regenerates Server/Models/Decoration/Decorations.go from
// Server/Data/EmpireItems/buildings.json and EmpireItemsMeta.json.
//
// Run from repository root:
//
//	go run ./Server/cmd/gendecorationwids
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

func main() {
	rootFlag := flag.String("root", "", "repository root (contains go.mod); default: walk up from cwd")
	flag.Parse()

	root, err := resolveRoot(*rootFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	buildingsPath := filepath.Join(root, "Server", "Data", "EmpireItems", "buildings.json")
	metaPath := filepath.Join(root, "Server", "Data", "EmpireItemsMeta.json")
	outPath := filepath.Join(root, "Server", "Models", "Decoration", "Decorations.go")

	buildingsRaw, err := os.ReadFile(buildingsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, buildingsPath, err)
		os.Exit(1)
	}
	metaRaw, err := os.ReadFile(metaPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, metaPath, err)
		os.Exit(1)
	}

	var buildings []struct {
		WodID float64 `json:"wodID"` // JSON number
		Type  string  `json:"type"`
	}
	if err := json.Unmarshal(buildingsRaw, &buildings); err != nil {
		fmt.Fprintln(os.Stderr, "buildings:", err)
		os.Exit(1)
	}
	var meta struct {
		CastleItemXMLVersion string `json:"castleItemXMLVersion"`
	}
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		fmt.Fprintln(os.Stderr, "meta:", err)
		os.Exit(1)
	}

	wids := make([]int, 0, len(buildings))
	for _, b := range buildings {
		if strings.EqualFold(strings.TrimSpace(b.Type), "deco") {
			wids = append(wids, int(b.WodID))
		}
	}
	slices.Sort(wids)

	var sb strings.Builder
	sb.WriteString(header(meta.CastleItemXMLVersion))
	sb.WriteString("\nvar DecorationWIDs = []int{\n")
	const perLine = 16
	for i := 0; i < len(wids); i += perLine {
		sb.WriteString("\t")
		end := i + perLine
		if end > len(wids) {
			end = len(wids)
		}
		for j := i; j < end; j++ {
			if j > i {
				sb.WriteString(", ")
			}
			sb.WriteString(strconv.Itoa(wids[j]))
		}
		sb.WriteString(",\n")
	}
	sb.WriteString("}\n")
	sb.WriteString(generatedTail())

	if err := os.WriteFile(outPath, []byte(sb.String()), 0644); err != nil {
		fmt.Fprintln(os.Stderr, outPath, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d WIDs, version %s)\n", outPath, len(wids), meta.CastleItemXMLVersion)
}

func header(version string) string {
	return fmt.Sprintf(`package decoration

import (
	"encoding/json"
	"strings"
	"sync"

	"CitadelDesktop/Server/Data"
	"CitadelDesktop/Server/Models/Castle"
)

// DecorationCatalogVersion matches Server/Data/EmpireItemsMeta.json (castleItemXMLVersion).
// Regenerate: go run ./Server/cmd/gendecorationwids
const DecorationCatalogVersion = %s

// DecorationWIDs lists every building wodID with type "deco" in EmpireItems/buildings.json (sorted).
`, strconv.Quote(version))
}

func generatedTail() string {
	jw := "`json:\"wodID\"`"
	jn := "`json:\"name\"`"
	jt := "`json:\"type\"`"
	var b strings.Builder
	b.WriteString("\nvar (\n\tdecorationWIDOnce     sync.Once\n\tdecorationWIDLookup map[int]struct{}\n)\n\n")
	b.WriteString("func decorationWIDInit() {\n\tdecorationWIDOnce.Do(func() {\n")
	b.WriteString("\t\tdecorationWIDLookup = make(map[int]struct{}, len(DecorationWIDs))\n")
	b.WriteString("\t\tfor _, id := range DecorationWIDs {\n\t\t\tdecorationWIDLookup[id] = struct{}{}\n\t\t}\n\t})\n}\n\n")
	b.WriteString("var (\n\tdecoNamesOnce sync.Once\n\tdecoNameByWID map[int]string\n\tdecoNamesErr  error\n)\n\n")
	b.WriteString("func loadDecoNamesFromBuildings() {\n\tdecoNamesOnce.Do(func() {\n")
	b.WriteString("\t\traw, err := data.ReadEmpireItemsSection(\"buildings\")\n")
	b.WriteString("\t\tif err != nil {\n\t\t\tdecoNamesErr = err\n\t\t\treturn\n\t\t}\n")
	b.WriteString("\t\tvar rows []struct {\n\t\t\tWodID int    " + jw + "\n")
	b.WriteString("\t\t\tName  string " + jn + "\n")
	b.WriteString("\t\t\tType  string " + jt + "\n\t\t}\n")
	b.WriteString("\t\tif err := json.Unmarshal(raw, &rows); err != nil {\n\t\t\tdecoNamesErr = err\n\t\t\treturn\n\t\t}\n")
	b.WriteString("\t\tdecoNameByWID = make(map[int]string)\n")
	b.WriteString("\t\tfor _, r := range rows {\n\t\t\tif strings.EqualFold(r.Type, \"deco\") {\n")
	b.WriteString("\t\t\t\tdecoNameByWID[r.WodID] = r.Name\n\t\t\t}\n\t\t}\n\t})\n}\n\n")
	b.WriteString("// IsKnownDecorationWID reports whether wid is listed as a decoration in EmpireItems buildings.json.\n")
	b.WriteString("func IsKnownDecorationWID(wid int) bool {\n\tdecorationWIDInit()\n\t_, ok := decorationWIDLookup[wid]\n\treturn ok\n}\n\n")
	b.WriteString("// DecorationDisplayName returns the display name from buildings.json for a decoration WID, if present.\n")
	b.WriteString("func DecorationDisplayName(wid int) (string, bool) {\n\tloadDecoNamesFromBuildings()\n")
	b.WriteString("\tif decoNamesErr != nil {\n\t\treturn \"\", false\n\t}\n\ts, ok := decoNameByWID[wid]\n\treturn s, ok\n}\n\n")
	b.WriteString("// IsEssentialCastleStructureByName matches core / production / defense buildings that must not be\n")
	b.WriteString("// bulk-removed as \"decorations\". Cosmetic items typically do not match these substrings.\n")
	b.WriteString("func IsEssentialCastleStructureByName(name string) bool {\n\tn := strings.ToLower(strings.TrimSpace(name))\n")
	b.WriteString("\tif n == \"\" || n == \"unknown\" {\n\t\treturn true\n\t}\n")
	b.WriteString("\tessentials := []string{\n")
	b.WriteString("\t\t\"barracks\", \"tower\", \"wall\", \"gate\", \"moat\", \"keep\", \"hospital\", \"stables\",\n")
	b.WriteString("\t\t\"farm\", \"woodcutter\", \"quarry\", \"mine\", \"mill\", \"market\", \"storehouse\", \"dwelling\",\n")
	b.WriteString("\t\t\"academy\", \"temple\", \"winery\", \"bakery\", \"apiary\", \"armory\", \"drill ground\",\n")
	b.WriteString("\t\t\"training grounds\", \"headquarters\", \"field kitchen\", \"ballista\", \"flame tower\",\n")
	b.WriteString("\t\t\"wood stock\", \"stone stock\", \"vault\", \"cartographer\", \"town house\", \"townhouse\",\n")
	b.WriteString("\t\t\"relic woodcutter\", \"relic quarry\", \"relic mine\", \"relic farmstead\", \"relic mill\",\n")
	b.WriteString("\t\t\"construction crane\", \"watchtower\", \"sawmill\", \"brickworks\", \"foundry\", \"glassworks\",\n")
	b.WriteString("\t\t\"estate\", \"granary\", \"workshop\", \"forge\", \"furnace\", \"kiln\", \"smelter\",\n\t}\n")
	b.WriteString("\tfor _, e := range essentials {\n\t\tif strings.Contains(n, e) {\n\t\t\treturn true\n\t\t}\n\t}\n\treturn false\n}\n\n")
	b.WriteString("// IsDecorationPickupCandidateWID is true when wid is in the decoration WID list, or when castle\n")
	b.WriteString("// building metadata suggests a non-essential cosmetic (fallback for newer WIDs).\n")
	b.WriteString("func IsDecorationPickupCandidateWID(wid int) bool {\n\tif IsKnownDecorationWID(wid) {\n\t\treturn true\n\t}\n")
	b.WriteString("\tinfo := castle.GetBuildingInfo(wid)\n\tif info.Name == \"Unknown\" {\n\t\treturn false\n\t}\n")
	b.WriteString("\treturn !IsEssentialCastleStructureByName(info.Name)\n}\n\n")
	b.WriteString("// DecorationSOBBlockedWID is true for building type IDs the server rejects for EmpireEx sob pickup (e.g. status 61).\n")
	b.WriteString("// Do not use IsDecorationPickupCandidateWID here: many live decorations share generic WIDs (e.g. 201 / \"Tower\") that\n")
	b.WriteString("// must still be cleared off preset tiles.\n")
	b.WriteString("func DecorationSOBBlockedWID(wid int) bool {\n\tswitch wid {\n")
	b.WriteString("\tcase 756, 1422, 2027: // construction yard, hall of legends, mead distillery — observed SOB 61\n")
	b.WriteString("\t\treturn true\n\tdefault:\n\t\treturn false\n\t}\n}\n")
	return b.String()
}

func resolveRoot(flagRoot string) (string, error) {
	if flagRoot != "" {
		abs, err := filepath.Abs(flagRoot)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil {
			return "", fmt.Errorf("-root %q: go.mod not found", abs)
		}
		return abs, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from cwd; use -root")
		}
		dir = parent
	}
}
