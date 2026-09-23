package Localization

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Source-defined keys are the translation contract. New producers cannot ship
// a keyed descriptor with a missing or different English template silently.
func TestProducerTemplatesExistInEnglishCatalog(t *testing.T) {
	raw, err := os.ReadFile("en.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog map[string]string
	if err = json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	dynamicRaw, err := os.ReadFile("dynamic.json")
	if err != nil {
		t.Fatal(err)
	}
	used := map[string]string{}
	if err = json.Unmarshal(dynamicRaw, &used); err != nil {
		t.Fatal(err)
	}
	checked := 0
	err = filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "New" {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "Localization" {
				return true
			}
			keyNode, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				var keyText, fallbackText bytes.Buffer
				printer.Fprint(&keyText, token.NewFileSet(), call.Args[0])
				printer.Fprint(&fallbackText, token.NewFileSet(), call.Args[1])
				allowed := strings.HasSuffix(filepath.ToSlash(path), "Telemetry/Store.go") && ((keyText.String() == `"server.telemetry.channel." + channel.ID + ".label"` && fallbackText.String() == "channel.Label") || (keyText.String() == `"server.telemetry.channel." + channel.ID + ".description"` && fallbackText.String() == "channel.Description"))
				if !allowed {
					t.Errorf("%s: unintended dynamic localization key", path)
				}
				return true
			}
			textNode, ok := call.Args[1].(*ast.BasicLit)
			if !ok {
				t.Errorf("%s: rendered fallback passed to keyed descriptor", path)
				return true
			}
			key, _ := strconv.Unquote(keyNode.Value)
			text, _ := strconv.Unquote(textNode.Value)
			if strings.HasPrefix(strings.TrimSpace(text), ":") || strings.HasPrefix(strings.TrimSpace(text), ";") {
				t.Errorf("%s: leading separator in %s", path, key)
			}
			if catalog[key] != text {
				t.Errorf("%s: missing or mismatched template %s", path, key)
			}
			used[key] = text
			checked++
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range catalog {
		if used[key] != value {
			t.Errorf("obsolete or mismatched English template: %s", key)
		}
	}
	if checked == 0 {
		t.Fatal("no producer templates checked")
	}
}
