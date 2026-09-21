// presentation-audit inventories visible producer fields and helper calls.
// This complements catalog parity: a message never given a descriptor cannot
// appear in a catalog-key audit. Missing entries are work, not coverage claims.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type source struct {
	path string
	raw  []byte
	file *ast.File
	set  *token.FileSet
}

func (s source) text(n ast.Node) string {
	if n == nil {
		return ""
	}
	return string(s.raw[s.set.Position(n.Pos()).Offset:s.set.Position(n.End()).Offset])
}

type helper struct {
	parameter, fixed int
	descriptors      bool
	kind             string
}
type entry struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Producer   string `json:"producer"`
	Field      string `json:"field"`
	Expression string `json:"expression"`
	Status     string `json:"status"`
}

func main() {
	if len(os.Args) != 2 {
		panic("usage: go run ./Server/Localization/cmd/presentation-audit <repository-root>")
	}
	root := os.Args[1]
	sources := []source{}
	helpers := map[string]helper{}
	err := filepath.Walk(filepath.Join(root, "Server"), func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		set := token.NewFileSet()
		file, err := parser.ParseFile(set, p, raw, 0)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		s := source{rel, raw, file, set}
		sources = append(sources, s)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
				continue
			}
			kind := s.text(fn.Type.Results.List[0].Type)
			if kind != "Intent.Step" && kind != "Step" && kind != "Decision" {
				continue
			}
			h := helper{parameter: -1, kind: kind}
			index := 0
			for _, field := range fn.Type.Params.List {
				if _, ok := field.Type.(*ast.Ellipsis); ok {
					h.descriptors = strings.Contains(s.text(field.Type), "Localization.Message")
					continue
				}
				for _, name := range field.Names {
					if s.text(field.Type) == "string" && (name.Name == "name" || name.Name == "label" || name.Name == "detail") {
						if h.parameter < 0 {
							h.parameter = index
						}
					}
					index++
				}
			}
			h.fixed = index
			if h.parameter >= 0 {
				helpers[fn.Name.Name] = h
			}
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	fields := map[string][]string{"Intent.Plan": {"Summary"}, "Plan": {"Summary"}, "Decision": {"Detail", "FailureDetail"}, "State.AutomationState": {"Detail", "LastError"}, "FailurePresentation": {"Message", "Explanation", "Recovery"}, "Channel": {"Label", "Description"}, "featureActivity": {"detail"}, "Intent.Step": {"Name"}, "Step": {"Name"}}
	entries := []entry{}
	covered := 0
	for _, s := range sources {
		stack := []ast.Node{}
		ast.Inspect(s.file, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			var parent ast.Node
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			stack = append(stack, n)
			emit := func(producer, field string, value ast.Expr, status string) {
				if status == "descriptor" {
					covered++
					return
				}
				entries = append(entries, entry{s.path, s.set.Position(value.Pos()).Line, producer, field, s.text(value), status})
			}
			switch node := n.(type) {
			case *ast.CompositeLit:
				kind := s.text(node.Type)
				if node.Type == nil {
					if outer, ok := parent.(*ast.CompositeLit); ok {
						if array, ok := outer.Type.(*ast.ArrayType); ok {
							kind = s.text(array.Elt)
						}
					}
				}
				allowed := fields[kind]
				if len(allowed) == 0 {
					break
				}
				values := map[string]ast.Expr{}
				for _, elt := range node.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						values[s.text(kv.Key)] = kv.Value
					}
				}
				for _, field := range allowed {
					value, ok := values[field]
					if !ok {
						continue
					}
					status := "missing"
					if values[field+"Descriptor"] != nil || field == "detail" && values["descriptor"] != nil {
						status = "descriptor"
					}
					emit(kind, field, value, status)
				}
			case *ast.AssignStmt:
				for i, lhs := range node.Lhs {
					if i >= len(node.Rhs) {
						continue
					}
					index, ok := lhs.(*ast.IndexExpr)
					if !ok {
						continue
					}
					name := s.text(index.X)
					if name != "details" && !strings.HasSuffix(name, ".Details") {
						continue
					}
					descriptorName := "detailDescriptors"
					if name != "details" {
						descriptorName = strings.TrimSuffix(name, ".Details") + ".DetailsDescriptors"
					}
					status := "missing"
					if block, ok := parent.(*ast.BlockStmt); ok {
						for j, statement := range block.List {
							if statement != node || j+1 >= len(block.List) {
								continue
							}
							next, ok := block.List[j+1].(*ast.AssignStmt)
							if !ok {
								continue
							}
							for _, target := range next.Lhs {
								if other, ok := target.(*ast.IndexExpr); ok && s.text(other.X) == descriptorName && s.text(other.Index) == s.text(index.Index) {
									status = "descriptor"
								}
							}
						}
					}
					emit("map assignment", s.text(lhs), node.Rhs[i], status)
				}
			case *ast.CallExpr:
				id, ok := node.Fun.(*ast.Ident)
				if !ok {
					break
				}
				h, ok := helpers[id.Name]
				if !ok || h.parameter >= len(node.Args) {
					break
				}
				status := "missing"
				if h.descriptors && len(node.Args) > h.fixed {
					status = "descriptor"
				}
				if selector, ok := parent.(*ast.SelectorExpr); ok && selector.Sel.Name == "WithNameDescriptor" {
					status = "descriptor"
				}
				emit(id.Name, "helper text", node.Args[h.parameter], status)
			}
			return true
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].File != entries[j].File {
			return entries[i].File < entries[j].File
		}
		if entries[i].Line != entries[j].Line {
			return entries[i].Line < entries[j].Line
		}
		return entries[i].Field < entries[j].Field
	})
	result := struct {
		SchemaVersion     int     `json:"schemaVersion"`
		DescriptorSites   int     `json:"descriptorSites"`
		UnclassifiedSites int     `json:"unclassifiedSites"`
		Limitations       string  `json:"limitations"`
		Entries           []entry `json:"entries"`
	}{1, covered, len(entries), "Syntax inventory only: entries include forwarding boundaries and finite telemetry registries that require explicit classification; non-map assignments and opaque third-party/history text need separate audit. Descriptor presence does not prove translated pack coverage or official noun provenance.", entries}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
