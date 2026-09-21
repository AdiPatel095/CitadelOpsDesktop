package Localization

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestWrappedErrorContextOnlyUsedForTrailingCause(t *testing.T) {
	count := 0
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
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
		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok || len(c.Args) != 2 {
				return true
			}
			sel, ok := c.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "WithError" {
				return true
			}
			descriptor, ok := c.Args[1].(*ast.CallExpr)
			if !ok {
				return true
			}
			ds, ok := descriptor.Fun.(*ast.SelectorExpr)
			if !ok || ds.Sel.Name != "ErrorContext" {
				return true
			}
			original, ok := c.Args[0].(*ast.CallExpr)
			if !ok || len(original.Args) == 0 {
				t.Errorf("%s: context wrapper lacks original format", path)
				return true
			}
			literal, ok := original.Args[0].(*ast.BasicLit)
			if !ok {
				t.Errorf("%s: context wrapper format must be static", path)
				return true
			}
			format, _ := strconv.Unquote(literal.Value)
			if !strings.HasSuffix(format, "%w") || strings.Count(format, "%w") != 1 {
				t.Errorf("%s: prefix/infix cause requires whole-message template: %s", path, format)
			}
			count++
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no context wrappers checked")
	}
}
