// ffi_gen is a code generator that scans the Go standard library and
// generates FFI registration code for ALL exported symbols (functions,
// variables, constants, types). Run it whenever Go is upgraded.
//
// Usage: GOTOOLCHAIN=local go run .
// Output: ffi_*.go files in current directory.
//         Copy generated files to microlisp/ and rebuild.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// PkgInfo holds extracted package information.
type PkgInfo struct {
	ImportPath string
	Name       string
	Functions  []Sym
	Vars       []Sym
	Consts     []Sym
	Types      []Sym
}

type Sym struct {
	Name          string
	Kind          string // "func", "var", "const", "type"
	ConstType     string // for constants: "int", "float", "string", "bool"
	ConstVal      string // for constants: value as string
	ConstOverflow bool   // for constants: true if value overflows int64
}

var (
	goroot   string
	skipPkgs = map[string]bool{
		"unsafe":       true,
		"builtin":      true,
		"syscall":      true, // platform-specific
		"log/syslog":   true, // not available on Windows
		"runtime/race": true, // empty, no exported symbols
		"maps":         true, // generic functions only
		"slices":       true, // generic functions only
		"time/tzdata":  true, // empty, only embeds data
		"cmp":          true, // only has type constraints (Ordered)
	}
)

func main() {
	goroot = os.Getenv("GOROOT")
	if goroot == "" {
		out, err := exec.Command("go", "env", "GOROOT").Output()
		if err != nil {
			log.Fatal("failed to get GOROOT:", err)
		}
		goroot = strings.TrimSpace(string(out))
	}
	log.Printf("GOROOT: %s", goroot)

	packages, err := listStdPackages()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Found %d public stdlib packages", len(packages))

	var allInfos []PkgInfo
	for _, pkgPath := range packages {
		if skipPkgs[pkgPath] {
			continue
		}
		info, err := extractPackage(pkgPath)
		if err != nil {
			log.Printf("  SKIP %s: %v", pkgPath, err)
			continue
		}
		allInfos = append(allInfos, *info)
		nSyms := len(info.Functions) + len(info.Vars) + len(info.Consts) + len(info.Types)
		log.Printf("  OK   %s: %d syms (%d funcs, %d vars, %d consts, %d types)",
			pkgPath, nSyms, len(info.Functions), len(info.Vars), len(info.Consts), len(info.Types))
	}

	log.Printf("Successfully processed %d packages", len(allInfos))

	// Clean old generated files
	oldFiles, _ := filepath.Glob("ffi_*.go")
	for _, f := range oldFiles {
		os.Remove(f)
	}

	// Generate one file per package
	totalSymbols := 0
	for _, info := range allInfos {
		fpath := "ffi_" + safeFileName(info.ImportPath) + ".go"
		count, err := generatePkgFile(info, fpath)
		if err != nil {
			log.Printf("  GEN FAIL %s: %v", info.ImportPath, err)
			continue
		}
		totalSymbols += count
	}

	// Generate master init file
	if err := generateInitFile(allInfos); err != nil {
		log.Fatal(err)
	}

	log.Printf("Generated %d registration files, %d total symbols", len(allInfos), totalSymbols)
	log.Printf("Done! Copy all ffi_*.go to microlisp/ and rebuild.")
}

func listStdPackages() ([]string, error) {
	cmd := exec.Command("go", "list", "std")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list std: %w", err)
	}

	var pkgs []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "unsafe" || line == "builtin" {
			continue
		}
		if strings.HasPrefix(line, "internal/") ||
			strings.HasPrefix(line, "vendor/") ||
			strings.Contains(line, "/internal/") ||
			strings.HasSuffix(line, "/internal") {
			continue
		}
		pkgs = append(pkgs, line)
	}
	return pkgs, nil
}

func extractPackage(pkgPath string) (*PkgInfo, error) {
	pkgDir := filepath.Join(goroot, "src", filepath.FromSlash(pkgPath))
	if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("source directory not found")
	}

	// Get the list of files that would be built for this platform
	// using `go list -json` which respects build tags
	buildFiles, err := getBuildFiles(pkgPath)
	if err != nil {
		// Fallback: parse all files
		return extractAllFiles(pkgDir, pkgPath)
	}

	fset := token.NewFileSet()
	var allFiles []*ast.File
	var pkgName string

	for _, fpath := range buildFiles {
		f, err := parser.ParseFile(fset, fpath, nil, 0)
		if err != nil {
			continue
		}
		if pkgName == "" {
			pkgName = f.Name.Name
		}
		allFiles = append(allFiles, f)
	}

	if len(allFiles) == 0 {
		return nil, fmt.Errorf("no parseable Go files")
	}

	// Try type-checking first
	result, err := tryTypeCheck(pkgPath, fset, allFiles)
	if err != nil {
		// Fallback to AST extraction
		log.Printf("    type-check failed for %s, using AST fallback", pkgPath)
		return extractFromAST(pkgName, pkgPath, allFiles)
	}

	return result, nil
}

// goListOutput is the JSON output from `go list -json`.
type goListOutput struct {
	GoFiles    []string `json:"GoFiles"`
	CgoFiles   []string `json:"CgoFiles"`
	ImportPath string   `json:"ImportPath"`
	Name       string   `json:"Name"`
	Dir        string   `json:"Dir"`
}

// getBuildFiles returns Go source files that would be compiled for current platform.
func getBuildFiles(pkgPath string) ([]string, error) {
	cmd := exec.Command("go", "list", "-json", pkgPath)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list -json %s: %w", pkgPath, err)
	}

	// Parse JSON - may have multiple objects for multiple packages
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	var allGoFiles []string
	var currentJSON strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		currentJSON.WriteString(line + "\n")

		if strings.TrimSpace(line) == "}" {
			var info goListOutput
			if err := json.Unmarshal([]byte(currentJSON.String()), &info); err == nil {
				allGoFiles = append(allGoFiles, info.GoFiles...)
				allGoFiles = append(allGoFiles, info.CgoFiles...)
			}
		}
	}

	if len(allGoFiles) == 0 {
		return nil, fmt.Errorf("no GoFiles found from go list")
	}

	// Resolve to full paths
	pkgDir := filepath.Join(goroot, "src", filepath.FromSlash(pkgPath))
	var result []string
	for _, f := range allGoFiles {
		result = append(result, filepath.Join(pkgDir, f))
	}
	return result, nil
}

// extractAllFiles is the fallback: parses all non-test Go files.
func extractAllFiles(pkgDir, pkgPath string) (*PkgInfo, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, pkgDir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	var allFiles []*ast.File
	var pkgName string
	for name, pkg := range pkgs {
		if pkgName == "" || name == filepath.Base(pkgDir) {
			pkgName = name
			allFiles = make([]*ast.File, 0, len(pkg.Files))
			for _, f := range pkg.Files {
				allFiles = append(allFiles, f)
			}
		}
	}

	if len(allFiles) == 0 {
		return nil, fmt.Errorf("no Go files found")
	}

	result, err := tryTypeCheck(pkgPath, fset, allFiles)
	if err != nil {
		return extractFromAST(pkgName, pkgPath, allFiles)
	}
	return result, nil
}

func tryTypeCheck(pkgPath string, fset *token.FileSet, files []*ast.File) (*PkgInfo, error) {
	conf := types.Config{
		Importer: importer.ForCompiler(fset, "source", nil),
	}
	info := &types.Info{
		Types:     make(map[ast.Expr]types.TypeAndValue),
		Instances: make(map[*ast.Ident]types.Instance),
	}
	typedPkg, err := conf.Check(pkgPath, fset, files, info)
	if err != nil {
		return nil, err
	}
	return extractFromTypes(pkgPath, typedPkg)
}

func extractFromTypes(pkgPath string, pkg *types.Package) (*PkgInfo, error) {
	scope := pkg.Scope()
	result := &PkgInfo{
		ImportPath: pkgPath,
		Name:       pkg.Name(),
	}

	for _, name := range scope.Names() {
		if !ast.IsExported(name) {
			continue
		}
		obj := scope.Lookup(name)
		if obj == nil {
			continue
		}

		switch obj := obj.(type) {
		case *types.Func:
			sig := obj.Type().(*types.Signature)
			if sig.TypeParams().Len() > 0 {
				continue // skip generic functions
			}
			result.Functions = append(result.Functions, Sym{
				Name: name,
				Kind: "func",
			})

		case *types.Var:
			result.Vars = append(result.Vars, Sym{
				Name: name,
				Kind: "var",
			})

		case *types.Const:
			basic, ok := obj.Type().(*types.Basic)
			if !ok {
				continue
			}
			kind := basic.Kind()
			if kind == types.UntypedNil {
				continue
			}
			constType := basicTypeKind(kind)
			if constType == "" {
				continue
			}
			valStr := obj.Val().String()
			overflow := false
			if constType == "int" {
				overflow = isIntOverflow(valStr)
			}
			if overflow {
				continue // skip constants that overflow int
			}
			result.Consts = append(result.Consts, Sym{
				Name:      name,
				Kind:      "const",
				ConstType: constType,
				ConstVal:  valStr,
			})

		case *types.TypeName:
			underlying := obj.Type().Underlying()
			typeKind := "type"
			// Skip type parameters (generic constraints like cmp.Ordered)
			if _, ok := obj.Type().(*types.TypeParam); ok {
				continue
			}
			switch underlying.(type) {
			case *types.Interface:
				typeKind = "interface"
			case *types.Struct:
				typeKind = "struct"
			case *types.Basic:
				typeKind = "basic"
			case *types.Array:
				typeKind = "array"
			case *types.Slice:
				typeKind = "slice"
			case *types.Map:
				typeKind = "map"
			case *types.Chan:
				typeKind = "chan"
			case *types.Pointer:
				typeKind = "pointer"
			}
			result.Types = append(result.Types, Sym{
				Name:      name,
				Kind:      "type",
				ConstType: typeKind,
			})
		}
	}

	sortSyms(result)
	return result, nil
}

func extractFromAST(pkgName, pkgPath string, files []*ast.File) (*PkgInfo, error) {
	result := &PkgInfo{
		ImportPath: pkgPath,
		Name:       pkgName,
	}

	for _, file := range files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil {
					continue // skip methods
				}
				if ast.IsExported(d.Name.Name) {
					result.Functions = append(result.Functions, Sym{
						Name: d.Name.Name,
						Kind: "func",
					})
				}

			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.ValueSpec:
						for _, name := range s.Names {
							if !ast.IsExported(name.Name) {
								continue
							}
							if d.Tok == token.CONST {
								result.Consts = append(result.Consts, Sym{
									Name: name.Name,
									Kind: "const",
								})
							} else {
								result.Vars = append(result.Vars, Sym{
									Name: name.Name,
									Kind: "var",
								})
							}
						}
					case *ast.TypeSpec:
						if ast.IsExported(s.Name.Name) {
							result.Types = append(result.Types, Sym{
								Name: s.Name.Name,
								Kind: "type",
							})
						}
					}
				}
			}
		}
	}

	// Deduplicate
	dedup(&result.Functions)
	dedup(&result.Vars)
	dedup(&result.Consts)
	dedup(&result.Types)
	sortSyms(result)
	return result, nil
}

func sortSyms(r *PkgInfo) {
	sort.Slice(r.Functions, func(i, j int) bool { return r.Functions[i].Name < r.Functions[j].Name })
	sort.Slice(r.Vars, func(i, j int) bool { return r.Vars[i].Name < r.Vars[j].Name })
	sort.Slice(r.Consts, func(i, j int) bool { return r.Consts[i].Name < r.Consts[j].Name })
	sort.Slice(r.Types, func(i, j int) bool { return r.Types[i].Name < r.Types[j].Name })
}

func dedup(syms *[]Sym) {
	seen := make(map[string]bool)
	result := (*syms)[:0]
	for _, s := range *syms {
		if !seen[s.Name] {
			seen[s.Name] = true
			result = append(result, s)
		}
	}
	*syms = result
}

func basicTypeKind(k types.BasicKind) string {
	switch {
	case k >= types.Bool && k <= types.UntypedBool:
		return "bool"
	case k >= types.Int && k <= types.Int64:
		return "int"
	case k >= types.Uint && k <= types.Uintptr:
		return "int"
	case k == types.Float32, k == types.Float64, k == types.UntypedFloat:
		return "float"
	case k == types.String, k == types.UntypedString:
		return "string"
	case k == types.UntypedRune, k == types.UntypedInt:
		return "int"
	}
	return ""
}

// isIntOverflow checks if an integer string representation would overflow
// a 64-bit int when used as a Go int value in a map[string]interface{}.
func isIntOverflow(valStr string) bool {
	if len(valStr) == 0 {
		return false
	}
	// Handle negative numbers
	neg := false
	s := valStr
	if valStr[0] == '-' {
		neg = true
		s = valStr[1:]
	}
	if len(s) == 0 {
		return false
	}
	// Max int64 = 9223372036854775807
	// Min int64 = -9223372036854775808
	if !neg {
		if len(s) > 19 {
			return true
		}
		if len(s) == 19 && s > "9223372036854775807" {
			return true
		}
	} else {
		if len(s) > 19 {
			return true
		}
		if len(s) == 19 && s > "9223372036854775808" {
			return true
		}
	}
	return false
}

func safeFileName(path string) string {
	return strings.ReplaceAll(path, "/", "_")
}

func generatePkgFile(info PkgInfo, outPath string) (int, error) {
	var b strings.Builder

	b.WriteString("// Code generated by microlisp/ffi_gen/main.go - DO NOT EDIT.\n")
	b.WriteString(fmt.Sprintf("// Auto-registered Go stdlib package: %s\n\n", info.ImportPath))
	b.WriteString("package microlisp\n\n")

	// Import alias - avoid conflicts with Go keywords and existing names
	importAlias := info.Name
	reserved := map[string]bool{
		"new": true, "make": true, "len": true, "cap": true,
		"append": true, "copy": true, "delete": true,
		"complex": true, "real": true, "imag": true,
		"close": true, "panic": true, "recover": true,
		"print": true, "println": true, "type": true,
		"list": true,    // conflicts with microlisp's list variable
		"reflect": true, // conflicts with reflect package import
		"cmp": true,     // conflicts with Go cmp type constraint usage
		"atomic": true,  // conflicts with sync/atomic generic types
	}
	if reserved[importAlias] {
		importAlias = importAlias + "Pkg"
	}

	// Import block
	b.WriteString("import (\n")
	b.WriteString(fmt.Sprintf("\t%s %q\n", importAlias, info.ImportPath))
	shouldRegisterTypes := !map[string]bool{
		"reflect": true, "cmp": true, "sync/atomic": true,
	}[info.ImportPath]
	if shouldRegisterTypes && len(info.Types) > 0 {
		b.WriteString("\t\"reflect\"\n")
	}
	b.WriteString(")\n\n")

	// Registration function
	funcName := "register_" + safeFileName(info.ImportPath)
	totalSymbols := len(info.Functions) + len(info.Vars) + len(info.Consts)

	b.WriteString(fmt.Sprintf("func %s() {\n", funcName))

	if totalSymbols == 0 && len(info.Types) == 0 {
		b.WriteString("\t// No exported symbols\n")
		b.WriteString("}\n\n")
		os.WriteFile(outPath, []byte(b.String()), 0644)
		return 0, nil
	}

	if totalSymbols > 0 {
		b.WriteString("\tregisterPackage(\n")
		b.WriteString(fmt.Sprintf("\t\t%q,\n", info.ImportPath))
		b.WriteString("\t\tmap[string]interface{}{\n")

		for _, sym := range info.Functions {
			b.WriteString(fmt.Sprintf("\t\t\t%q: %s.%s,\n", sym.Name, importAlias, sym.Name))
		}
		for _, sym := range info.Vars {
			b.WriteString(fmt.Sprintf("\t\t\t%q: %s.%s,\n", sym.Name, importAlias, sym.Name))
		}
		for _, sym := range info.Consts {
			b.WriteString(fmt.Sprintf("\t\t\t%q: %s.%s,\n", sym.Name, importAlias, sym.Name))
		}

		b.WriteString("\t\t},\n\t)\n")
	}

	// Type registrations - use (*T)(nil)).Elem() for all types
	// shouldRegisterTypes already computed above
	if shouldRegisterTypes && len(info.Types) > 0 {
		for _, sym := range info.Types {
			b.WriteString(fmt.Sprintf("\tregisterType(%q, %q, reflect.TypeOf((*%s.%s)(nil)).Elem())\n",
				info.ImportPath, sym.Name, importAlias, sym.Name))
		}
	}

	b.WriteString("}\n\n")

	os.WriteFile(outPath, []byte(b.String()), 0644)
	return totalSymbols + len(info.Types), nil
}

func generateInitFile(infos []PkgInfo) error {
	var b strings.Builder

	b.WriteString("// Code generated by microlisp/ffi_gen/main.go - DO NOT EDIT.\n")
	b.WriteString("// Master init: calls all generated registration functions.\n\n")
	b.WriteString("package microlisp\n\n")
	b.WriteString("func init() {\n")

	sorted := make([]PkgInfo, len(infos))
	copy(sorted, infos)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ImportPath < sorted[j].ImportPath })

	count := 0
	for _, info := range sorted {
		funcName := "register_" + safeFileName(info.ImportPath)
		nSyms := len(info.Functions) + len(info.Vars) + len(info.Consts) + len(info.Types)
		if nSyms > 0 {
			b.WriteString(fmt.Sprintf("\t%s() // %d symbols\n", funcName, nSyms))
			count++
		}
	}

	b.WriteString(fmt.Sprintf("\t// Total: %d packages, auto-generated from Go %s stdlib\n", count, getGoVersion()))
	b.WriteString("}\n")

	return os.WriteFile("ffi_init.go", []byte(b.String()), 0644)
}

func getGoVersion() string {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
