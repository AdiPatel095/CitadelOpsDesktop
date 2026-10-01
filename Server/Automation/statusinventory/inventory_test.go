package statusinventory

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite the automation status golden inventory")

type listedPackage struct {
	ImportPath, Dir, Export string
	GoFiles                 []string
	ImportMap               map[string]string
	Error                   *struct{ Err string }
}
type checkedPackage struct {
	pkg   *types.Package
	info  *types.Info
	files []*ast.File
}
type sourceImporter struct {
	root, module string
	fset         *token.FileSet
	listed       map[string]listedPackage
	checked      map[string]*checkedPackage
	loading      map[string]bool
	external     types.Importer
	sizes        types.Sizes
}

func loadModule(root, goos string) (*sourceImporter, error) {
	goExe, err := exec.LookPath("go")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, goExe, "list", "-e", "-json", "-export", "-deps", "./...")
	cmd.Dir = root
	// Replace rather than append: caller GOOS/CGO must not affect the scan.
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GOOS=") && !strings.HasPrefix(e, "CGO_ENABLED=") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Env = append(cmd.Env, "GOOS="+goos, "CGO_ENABLED=0")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list (%s): %w", goos, err)
	}
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil, err
	}
	var module string
	for _, line := range strings.Split(string(mod), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			module = strings.Trim(fields[1], "\"")
			break
		}
	}
	if module == "" {
		return nil, fmt.Errorf("no module directive in %s", root)
	}
	s := &sourceImporter{root: root, module: module, fset: token.NewFileSet(), listed: map[string]listedPackage{}, checked: map[string]*checkedPackage{}, loading: map[string]bool{}, sizes: types.SizesFor("gc", targetArch())}
	dec := json.NewDecoder(strings.NewReader(string(output)))
	for {
		var p listedPackage
		err := dec.Decode(&p)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if p.Error != nil {
			return nil, fmt.Errorf("go list (%s) %s: %s", goos, p.ImportPath, p.Error.Err)
		}
		s.listed[p.ImportPath] = p
	}
	s.external = importer.ForCompiler(s.fset, "gc", func(path string) (io.ReadCloser, error) {
		p, ok := s.listed[path]
		if !ok || p.Export == "" {
			return nil, fmt.Errorf("missing export data for %s", path)
		}
		return os.Open(p.Export)
	})
	paths := make([]string, 0)
	for path := range s.listed {
		if s.isModule(path) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		if _, err := s.Import(path); err != nil {
			return nil, fmt.Errorf("type-check (%s): %w", goos, err)
		}
	}
	return s, nil
}

func targetArch() string {
	// go list inherits GOARCH, including any explicitly requested cross architecture.
	if arch := os.Getenv("GOARCH"); arch != "" {
		return arch
	}
	return runtime.GOARCH
}

func (s *sourceImporter) isModule(path string) bool {
	return path == s.module || strings.HasPrefix(path, s.module+"/")
}
func (s *sourceImporter) Import(path string) (*types.Package, error) {
	if p := s.checked[path]; p != nil {
		return p.pkg, nil
	}
	if !s.isModule(path) {
		return s.external.Import(path)
	}
	if s.loading[path] {
		return nil, fmt.Errorf("source import cycle: %s", path)
	}
	p, ok := s.listed[path]
	if !ok {
		return nil, fmt.Errorf("missing module package %s", path)
	}
	s.loading[path] = true
	defer delete(s.loading, path)
	var files []*ast.File
	for _, name := range p.GoFiles {
		f, err := parser.ParseFile(s.fset, filepath.Join(p.Dir, name), nil, 0)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
	config := types.Config{Importer: packageImporter{s, p.ImportMap}, Sizes: s.sizes}
	pkg, err := config.Check(path, s.fset, files, info)
	if err != nil {
		return nil, err
	}
	s.checked[path] = &checkedPackage{pkg, info, files}
	return pkg, nil
}

type packageImporter struct {
	s       *sourceImporter
	imports map[string]string
}

func (p packageImporter) Import(path string) (*types.Package, error) {
	if mapped := p.imports[path]; mapped != "" {
		path = mapped
	}
	return p.s.Import(path)
}

type targetSpec struct{ pkg, name string }
type function struct {
	obj     *types.Func
	decl    *ast.FuncDecl
	pkg     *checkedPackage
	returns []*ast.ReturnStmt
	params  map[types.Object]int
	results []types.Object
	calls   []*ast.CallExpr
	escaped bool
}
type expression struct {
	expr   ast.Expr
	pkg    *checkedPackage
	result int
}
type write struct {
	site   string
	value  expression
	forced bool
}
type scanner struct {
	source      *sourceImporter
	targets     map[types.Object]bool
	targetTypes map[types.Type]int
	functions   map[*types.Func]*function
	locals      map[types.Object][]expression
	addressed   map[types.Object]bool
	parameters  map[types.Object]*function
	writes      []write
}
type values struct {
	statuses   map[string]bool
	unresolved bool
}

func emptyValues() values { return values{statuses: map[string]bool{}} }
func (v *values) merge(other values) {
	for status := range other.statuses {
		v.statuses[status] = true
	}
	v.unresolved = v.unresolved || other.unresolved
}

func newScanner(source *sourceImporter, specs []targetSpec) (*scanner, error) {
	s := &scanner{source: source, targets: map[types.Object]bool{}, targetTypes: map[types.Type]int{}, functions: map[*types.Func]*function{}, locals: map[types.Object][]expression{}, addressed: map[types.Object]bool{}, parameters: map[types.Object]*function{}}
	for _, spec := range specs {
		p := source.checked[source.module+"/"+spec.pkg]
		if p == nil {
			return nil, fmt.Errorf("target package moved: %s", spec.pkg)
		}
		obj := p.pkg.Scope().Lookup(spec.name)
		if obj == nil {
			return nil, fmt.Errorf("target type moved: %s.%s", spec.pkg, spec.name)
		}
		st, ok := obj.Type().Underlying().(*types.Struct)
		if !ok {
			return nil, fmt.Errorf("target is not a struct: %s.%s", spec.pkg, spec.name)
		}
		found := false
		for i := 0; i < st.NumFields(); i++ {
			field := st.Field(i)
			if field.Name() == "Status" {
				if !types.Identical(field.Type(), types.Typ[types.String]) {
					return nil, fmt.Errorf("target Status type moved: %s.%s", spec.pkg, spec.name)
				}
				s.targets[field] = true
				s.targetTypes[obj.Type()] = i
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("target field moved: %s.%s.Status", spec.pkg, spec.name)
		}
	}
	// All function objects must be registered before resolving call sites.
	for _, p := range source.checked {
		for _, file := range p.files {
			for _, decl := range file.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					continue
				}
				obj := p.info.Defs[fd.Name].(*types.Func)
				fn := &function{obj: obj, decl: fd, pkg: p, params: map[types.Object]int{}}
				sig := obj.Type().(*types.Signature)
				for i := 0; i < sig.Params().Len(); i++ {
					param := sig.Params().At(i)
					fn.params[param] = i
					if fd.Recv == nil {
						s.parameters[param] = fn
					}
				}
				for i := 0; i < sig.Results().Len(); i++ {
					fn.results = append(fn.results, sig.Results().At(i))
				}
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					if _, closure := n.(*ast.FuncLit); closure {
						return false
					}
					if ret, ok := n.(*ast.ReturnStmt); ok {
						fn.returns = append(fn.returns, ret)
					}
					return true
				})
				s.functions[obj] = fn
			}
		}
	}
	for _, p := range source.checked {
		for _, file := range p.files {
			s.indexFile(p, file)
		}
	}
	return s, nil
}

func unparen(e ast.Expr) ast.Expr {
	for {
		if p, ok := e.(*ast.ParenExpr); ok {
			e = p.X
		} else {
			return e
		}
	}
}
func staticObject(p *checkedPackage, e ast.Expr) *types.Func {
	e = unparen(e)
	switch e := e.(type) {
	case *ast.Ident:
		obj, _ := p.info.Uses[e].(*types.Func)
		return obj
	case *ast.SelectorExpr:
		if selection := p.info.Selections[e]; selection != nil {
			if _, iface := selection.Recv().Underlying().(*types.Interface); iface {
				return nil
			}
			obj, _ := selection.Obj().(*types.Func)
			return obj
		}
		obj, _ := p.info.Uses[e.Sel].(*types.Func)
		return obj
	case *ast.IndexExpr:
		return staticObject(p, e.X)
	case *ast.IndexListExpr:
		return staticObject(p, e.X)
	}
	return nil
}
func (s *scanner) isTarget(p *checkedPackage, e ast.Expr) bool {
	selector, ok := unparen(e).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	selection := p.info.Selections[selector]
	return selection != nil && s.targets[selection.Obj()]
}
func pair(rhs []ast.Expr, i int, p *checkedPackage) expression {
	if len(rhs) == 1 && i > 0 {
		return expression{rhs[0], p, i}
	}
	if i < len(rhs) {
		return expression{rhs[i], p, 0}
	}
	return expression{nil, p, 0}
}
func (s *scanner) addWrite(p *checkedPackage, fn string, lhs ast.Expr, rhs expression, forced bool) {
	pos := s.source.fset.Position(lhs.Pos())
	file, _ := filepath.Rel(s.source.root, pos.Filename)
	right := "<implicit>"
	if rhs.expr != nil {
		right = types.ExprString(rhs.expr)
	}
	site := filepath.ToSlash(file) + "|" + fn + "|" + types.ExprString(lhs) + " = " + right
	s.writes = append(s.writes, write{site, rhs, forced})
}
func (s *scanner) indexFile(p *checkedPackage, file *ast.File) {
	// Recognize direct callee syntax before classifying function value uses.
	direct := map[*ast.Ident]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			obj := staticObject(p, call.Fun)
			if fn := s.functions[obj]; fn != nil {
				fn.calls = append(fn.calls, call)
			}
			e := unparen(call.Fun)
			for {
				switch x := e.(type) {
				case *ast.IndexExpr:
					e = x.X
				case *ast.IndexListExpr:
					e = x.X
				default:
					goto callee
				}
			}
		callee:
			switch e := e.(type) {
			case *ast.Ident:
				direct[e] = true
			case *ast.SelectorExpr:
				direct[e.Sel] = true
			}
		}
		return true
	})
	var visit func(ast.Node, string, bool)
	visit = func(node ast.Node, name string, inFunction bool) {
		ast.Inspect(node, func(n ast.Node) bool {
			if n == nil {
				return true
			}
			if fd, ok := n.(*ast.FuncDecl); ok {
				fn := fd.Name.Name
				if fd.Recv != nil {
					fn = types.ExprString(fd.Recv.List[0].Type) + "." + fn
				}
				if fd.Body != nil {
					visit(fd.Body, fn, true)
				}
				return false
			}
			switch n := n.(type) {
			case *ast.Ident:
				if obj, ok := p.info.Uses[n].(*types.Func); ok && !direct[n] {
					if fn := s.functions[obj]; fn != nil {
						fn.escaped = true
					}
				}
				if inFunction {
					if obj, ok := p.info.Defs[n].(*types.Var); ok {
						if _, exists := s.locals[obj]; !exists {
							s.locals[obj] = nil
						}
					}
				}
			case *ast.AssignStmt:
				for i, lhs := range n.Lhs {
					rhs := pair(n.Rhs, i, p)
					if s.isTarget(p, lhs) {
						s.addWrite(p, name, lhs, rhs, n.Tok != token.ASSIGN)
					}
					if ident, ok := lhs.(*ast.Ident); ok && inFunction {
						obj := p.info.ObjectOf(ident)
						if obj != nil {
							if n.Tok == token.ASSIGN || n.Tok == token.DEFINE {
								s.locals[obj] = append(s.locals[obj], rhs)
							} else {
								s.addressed[obj] = true
							}
						}
					}
				}
			case *ast.ValueSpec:
				if inFunction {
					for i, ident := range n.Names {
						obj := p.info.Defs[ident]
						if len(n.Values) > 0 {
							s.locals[obj] = append(s.locals[obj], pair(n.Values, i, p))
						}
					}
				}
			case *ast.UnaryExpr:
				if n.Op == token.AND {
					if s.isTarget(p, n.X) {
						s.addWrite(p, name, n.X, expression{n, p, 0}, true)
					}
					if ident, ok := unparen(n.X).(*ast.Ident); ok {
						s.addressed[p.info.ObjectOf(ident)] = true
					}
				}
			case *ast.RangeStmt:
				for _, lhs := range []ast.Expr{n.Key, n.Value} {
					if lhs == nil {
						continue
					}
					if s.isTarget(p, lhs) {
						s.addWrite(p, name, lhs, expression{n.X, p, 0}, true)
					}
					if ident, ok := lhs.(*ast.Ident); ok {
						s.addressed[p.info.ObjectOf(ident)] = true
					}
				}
			case *ast.CompositeLit:
				index, target := s.targetTypes[types.Unalias(p.info.TypeOf(n))]
				for i, elt := range n.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						if ident, ok := kv.Key.(*ast.Ident); ok && s.targets[p.info.Uses[ident]] {
							s.addWrite(p, name, kv.Key, expression{kv.Value, p, 0}, false)
						}
					} else if target && i == index {
						lhs := n.Type
						if lhs == nil {
							lhs = n
						}
						s.addWrite(p, name, lhs, expression{elt, p, 0}, true)
					}
				}
			}
			return true
		})
	}
	visit(file, "<package>", false)
}

func (s *scanner) resolve(e expression, depth int, seen map[any]bool) values {
	v := emptyValues()
	if e.expr == nil || depth > 10 || seen[e] {
		v.unresolved = true
		return v
	}
	seen[e] = true
	defer delete(seen, e)
	p := e.pkg
	expr := unparen(e.expr)
	if tv := p.info.Types[expr]; e.result == 0 && tv.Value != nil && tv.Value.Kind() == constant.String {
		v.statuses[constant.StringVal(tv.Value)] = true
		return v
	}
	if s.isTarget(p, expr) {
		return v
	}
	switch expr := expr.(type) {
	case *ast.Ident:
		obj := p.info.ObjectOf(expr)
		if obj == nil || seen[obj] || s.addressed[obj] {
			v.unresolved = true
			return v
		}
		seen[obj] = true
		defer delete(seen, obj)
		if fn := s.parameters[obj]; fn != nil {
			index := fn.params[obj]
			sig := fn.obj.Type().(*types.Signature)
			if fn.escaped || len(fn.calls) == 0 || (sig.Variadic() && index == sig.Params().Len()-1) {
				v.unresolved = true
				return v
			}
			for _, call := range fn.calls {
				caller := s.expressionPackage(call)
				if len(call.Args) == 1 {
					if _, tuple := caller.info.TypeOf(call.Args[0]).(*types.Tuple); tuple {
						v.merge(s.resolve(expression{call.Args[0], caller, index}, depth+1, seen))
						continue
					}
				}
				if index >= len(call.Args) {
					v.unresolved = true
					continue
				}
				v.merge(s.resolve(expression{call.Args[index], caller, 0}, depth+1, seen))
			}
			// Parameter reassignment within the function can also write a status.
			for _, assignment := range s.locals[obj] {
				v.merge(s.resolve(assignment, depth+1, seen))
			}
			return v
		}
		assignments, ok := s.locals[obj]
		if !ok || len(assignments) == 0 {
			v.unresolved = true
			return v
		}
		for _, assignment := range assignments {
			v.merge(s.resolve(assignment, depth+1, seen))
		}
		return v
	case *ast.CallExpr:
		fn := s.functions[staticObject(p, expr.Fun)]
		if fn == nil || len(fn.returns) == 0 {
			v.unresolved = true
			return v
		}
		for _, ret := range fn.returns {
			if len(ret.Results) == 0 {
				if e.result >= len(fn.results) || fn.results[e.result].Name() == "" {
					v.unresolved = true
					continue
				}
				obj := fn.results[e.result]
				if seen[obj] || s.addressed[obj] || len(s.locals[obj]) == 0 {
					v.unresolved = true
					continue
				}
				seen[obj] = true
				for _, assignment := range s.locals[obj] {
					v.merge(s.resolve(assignment, depth+1, seen))
				}
				delete(seen, obj)
			} else {
				v.merge(s.resolve(pair(ret.Results, e.result, fn.pkg), depth+1, seen))
			}
		}
		return v
	}
	v.unresolved = true
	return v
}
func (s *scanner) expressionPackage(e ast.Expr) *checkedPackage {
	for _, p := range s.source.checked {
		if _, ok := p.info.Types[e]; ok {
			return p
		}
	}
	panic("call expression has no checked package")
}

type inventory struct {
	Statuses []string `json:"statuses"`
}
type allowance struct {
	Site   string   `json:"site"`
	Values []string `json:"values"`
	Reason string   `json:"reason"`
}
type scanResult struct {
	statuses   map[string]bool
	unresolved map[string]bool
}

func (s *scanner) scan() scanResult {
	result := scanResult{map[string]bool{}, map[string]bool{}}
	for _, write := range s.writes {
		v := emptyValues()
		if !write.forced {
			v = s.resolve(write.value, 0, map[any]bool{})
		}
		for status := range v.statuses {
			result.statuses[status] = true
		}
		if write.forced || v.unresolved {
			result.unresolved[write.site] = true
		}
	}
	return result
}
func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func applyAllowlist(result scanResult, entries []allowance) (inventory, error) {
	bySite := map[string]allowance{}
	for _, entry := range entries {
		if _, exists := bySite[entry.Site]; exists {
			return inventory{}, fmt.Errorf("duplicate allowlist site: %s", entry.Site)
		}
		if len(entry.Values) == 0 || strings.TrimSpace(entry.Reason) == "" {
			return inventory{}, fmt.Errorf("allowlist needs values and reason: %s", entry.Site)
		}
		if !result.unresolved[entry.Site] {
			return inventory{}, fmt.Errorf("stale allowlist site: %s", entry.Site)
		}
		bySite[entry.Site] = entry
	}
	for _, site := range sortedKeys(result.unresolved) {
		entry, exists := bySite[site]
		if !exists {
			return inventory{}, fmt.Errorf("unresolved status write needs allowlist: %s\nall unresolved sites:\n%s", site, strings.Join(sortedKeys(result.unresolved), "\n"))
		}
		for _, status := range entry.Values {
			result.statuses[status] = true
		}
	}
	return inventory{sortedKeys(result.statuses)}, nil
}
func moduleRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", start)
		}
		dir = parent
	}
}
func TestAutomationStatusInventory(t *testing.T) {
	root, err := moduleRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	combined := scanResult{map[string]bool{}, map[string]bool{}}
	for _, goos := range []string{"linux", "windows"} {
		source, err := loadModule(root, goos)
		if err != nil {
			t.Fatal(err)
		}
		scanner, err := newScanner(source, []targetSpec{{"Server/Automation", "Decision"}, {"Server/State", "AutomationState"}})
		if err != nil {
			t.Fatal(err)
		}
		result := scanner.scan()
		for status := range result.statuses {
			combined.statuses[status] = true
		}
		for site := range result.unresolved {
			combined.unresolved[site] = true
		}
		t.Logf("%s: %d source packages, %d writes, %d unresolved sites", goos, len(source.checked), len(scanner.writes), len(result.unresolved))
	}
	data, err := os.ReadFile("testdata/allowlist.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []allowance
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	got, err := applyAllowlist(combined, entries)
	if err != nil {
		t.Fatal(err)
	}
	goldenPath := "testdata/automation-statuses.golden.json"
	if *update {
		data, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, append(data, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
	data, err = os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	var want inventory
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("status inventory changed; review writes and run go test ./Server/Automation/statusinventory -update\ngot: %v\nwant: %v", got.Statuses, want.Statuses)
	}
	t.Logf("inventory: %d statuses, %d reviewed allowlist entries", len(got.Statuses), len(entries))
}

func TestFixtureInventory(t *testing.T) {
	root, err := filepath.Abs("testdata/fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, goos := range []string{"linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			source, err := loadModule(root, goos)
			if err != nil {
				t.Fatal(err)
			}
			s, err := newScanner(source, []targetSpec{{"automation", "Decision"}, {"state", "AutomationState"}})
			if err != nil {
				t.Fatal(err)
			}
			result := s.scan()
			// F1-F9 are exact, separately from additional conservative cases.
			edges := map[string]bool{}
			for site := range result.unresolved {
				if strings.HasPrefix(site, "writers/edges.go|") {
					edges[site] = true
					delete(result.unresolved, site)
				}
			}
			edgeSites := []string{
				"writers/edges.go|edges|d.Status = addressed",
				"writers/edges.go|edges|d.Status = \"suffix\"",
				"writers/edges.go|edges|automation.Decision = \"unkeyed\"",
				"writers/edges.go|edges|(ast: <nil>){…} = \"implicit-unkeyed\"",
				"writers/edges.go|edges|cur.Status = []string{…}",
				"writers/edges.go|escaped|d.Status = status",
				"writers/edges.go|neverCalled|d.Status = status",
				"writers/edges.go|variadic|d.Status = statuses[0]",
				"writers/edges.go|edges|d.Status = h.status()",
				"writers/edges.go|edges|d.Status = local()",
			}
			sort.Strings(edgeSites)
			if got := sortedKeys(edges); !reflect.DeepEqual(got, edgeSites) {
				t.Fatalf("conservative sites: got %v; want %v", got, edgeSites)
			}
			want := []string{"branch-a", "branch-b", "hibernating", "named", "parallel", "parameter-a", "parameter-b", "tuple", "closure-a", "closure-b", "method", "named-result"}
			sort.Strings(want)
			if got := sortedKeys(result.statuses); !reflect.DeepEqual(got, want) {
				t.Fatalf("resolved statuses: got %v; want %v", got, want)
			}
			sites := []string{"writers/writers.go|writes|cur.Status = &cur.Status", "writers/writers.go|writes|d.Status = m[k]"}
			if got := sortedKeys(result.unresolved); !reflect.DeepEqual(got, sites) {
				t.Fatalf("unresolved sites: got %v; want %v", got, sites)
			}
			entries := []allowance{
				{sites[0], []string{"addressed"}, "Fixture pointer may be used to write addressed."},
				{sites[1], []string{"mapped"}, "Fixture map is restricted to mapped."},
			}
			if _, err := applyAllowlist(result, nil); err == nil {
				t.Fatal("missing allowlist must fail")
			}
			got, err := applyAllowlist(result, entries)
			if err != nil {
				t.Fatal(err)
			}
			want = append(want, "addressed", "mapped")
			sort.Strings(want)
			if !reflect.DeepEqual(got.Statuses, want) {
				t.Fatalf("allowlisted inventory: got %v; want %v", got.Statuses, want)
			}
			for _, tc := range []struct {
				name    string
				entries []allowance
			}{
				{"stale", append(append([]allowance{}, entries...), allowance{"stale", []string{"stale"}, "Unused site."})},
				{"duplicate", append(append([]allowance{}, entries...), entries[0])},
				{"missing values", []allowance{{sites[0], nil, "Has no reviewed values."}}},
				{"missing reason", []allowance{{sites[0], []string{"addressed"}, " "}}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if _, err := applyAllowlist(result, tc.entries); err == nil {
						t.Fatal("invalid allowlist must fail")
					}
				})
			}
		})
	}
}
