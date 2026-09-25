package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: go run ./Server/Localization/cmd/catalog <repository-root>")
	}
	root := os.Args[1]
	path := root + "/Server/Localization/en.json"
	catalog := map[string]string{}
	raw, err := os.ReadFile(root + "/Server/Localization/dynamic.json")
	if err != nil {
		panic(err)
	}
	if err = json.Unmarshal(raw, &catalog); err != nil {
		panic(err)
	}
	err = filepath.Walk(root+"/Server", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, e := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if e != nil {
			return e
		}
		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok || len(c.Args) != 3 {
				return true
			}
			sel, ok := c.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "New" {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok || ident.Name != "Localization" {
				return true
			}
			key, ok := c.Args[0].(*ast.BasicLit)
			if !ok {
				return true
			}
			value, ok := c.Args[1].(*ast.BasicLit)
			if !ok {
				return true
			}
			k, _ := strconv.Unquote(key.Value)
			v, _ := strconv.Unquote(value.Value)
			catalog[k] = v
			return true
		})
		return nil
	})
	if err != nil {
		panic(err)
	}
	raw, err = json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile(path, append(raw, '\n'), 0644); err != nil {
		panic(err)
	}
}
