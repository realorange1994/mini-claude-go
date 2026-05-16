package microlisp

import (
	"fmt"
	"strings"
)

func builtinMakePackage(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-package: need package name")
	}
	name := primaryValue(args[0]).str
	pkg := makePackage(name)
	return vpkg(pkg), nil
}

func builtinInPackage(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("in-package: need package name")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return nil, fmt.Errorf("in-package: package not found")
	}
	currentPackage = pkg
	globalEnv.Set("*package*", vpkg(pkg))
	return vpkg(pkg), nil
}

func builtinFindPackage(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("find-package: need package name")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return vnil(), nil
	}
	return vpkg(pkg), nil
}

func builtinIntern(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("intern: need symbol name")
	}
	name := strings.ToUpper(primaryValue(args[0]).str)
	pkg := currentPackage
	if len(args) >= 2 && !isNil(args[1]) {
		pkg = resolvePackageFromDesignator(args[1])
		if pkg == nil {
			return nil, fmt.Errorf("intern: package not found")
		}
	}
	return internSymbol(name, pkg), nil
}

func builtinExport(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("export: need symbol or list of symbols")
	}
	pkg := currentPackage
	if len(args) >= 2 {
		pkg2 := resolvePackageFromDesignator(primaryValue(args[1]))
		if pkg2 != nil {
			pkg = pkg2
		}
	}
	// CL spec: first arg can be a symbol, string, or list of symbols/strings
	var syms []*Value
	first := primaryValue(args[0])
	if first.typ == VPair {
		syms = toSlice(first)
	} else {
		syms = []*Value{first}
	}
	var lastSym *Value
	for _, s := range syms {
		sym := primaryValue(s)
		var symName string
		if sym.typ == VSym {
			symName = sym.str
			// Strip keyword prefix
			if isKeyword(symName) {
				symName = keywordName(symName)
			}
		} else if sym.typ == VStr {
			// CL spec: strings are interned as symbols in the package
			symName = sym.str
			// Intern the string as a symbol in the package
			if existing, ok := pkg.symbols[symName]; ok {
				sym = existing
			} else {
				sym = internSymbol(symName, pkg)
				pkg.symbols[symName] = sym
			}
		} else {
			return nil, fmt.Errorf("export: need symbol or string, got %v", sym.typ)
		}
		pkg.exports[symName] = true
		// Also intern the symbol in the package
		if _, ok := pkg.symbols[symName]; !ok {
			pkg.symbols[symName] = sym
		}
		lastSym = sym
	}
	if len(syms) == 1 {
		return lastSym, nil
	}
	return vbool(true), nil
}

func builtinFindSymbol(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("find-symbol: need string name")
	}
	name := strings.ToUpper(primaryValue(args[0]).str)
	pkg := currentPackage
	if len(args) >= 2 && !isNil(args[1]) {
		pkg = resolvePackageFromDesignator(args[1])
		if pkg == nil {
			// Designator is not a valid package
			return multiVal(vnil(), vnil()), nil
		}
	}
	// Check internal symbols (including exported = external)
	if sym, ok := pkg.symbols[name]; ok {
		if pkg.exports[name] {
			return multiVal(sym, vsym("EXTERNAL")), nil
		}
		return multiVal(sym, vsym("INTERNAL")), nil
	}
	// Check inherited via use-list
	for _, used := range pkg.used {
		if sym, ok := used.symbols[name]; ok && used.exports[name] {
			return multiVal(sym, vsym("INHERITED")), nil
		}
	}
	return multiVal(vnil(), vnil()), nil
}

func builtinFindAllSymbols(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("find-all-symbols: need string name")
	}
	name := strings.ToUpper(primaryValue(args[0]).str)
	var result *Value
	for _, pkg := range packages {
		if sym, ok := pkg.symbols[name]; ok {
			result = cons(sym, result)
		}
	}
	if result == nil {
		return vnil(), nil
	}
	return result, nil
}

func builtinKeywordP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	if args[0].typ == VSym {
		kPkg := findPackage("KEYWORD")
		if kPkg != nil {
			if _, ok := kPkg.symbols[args[0].str]; ok {
				return vbool(true), nil
			}
		}
	}
	return vbool(false), nil
}

func builtinSymbolPackage(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VSym {
		return vnil(), nil
	}
	name := args[0].str
	if isKeyword(name) {
		kPkg := findPackage("KEYWORD")
		if kPkg != nil {
			return vpkg(kPkg), nil
		}
		return vnil(), nil
	}
	// Find which package this symbol belongs to
	for _, pkg := range packages {
		if _, ok := pkg.symbols[name]; ok {
			return vpkg(pkg), nil
		}
	}
	return vnil(), nil
}

func builtinPackageName(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("package-name: need package designator")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return nil, fmt.Errorf("package-name: package not found")
	}
	return vstr(pkg.name), nil
}

func builtinListAllPackages(args []*Value) (*Value, error) {
	result := vnil()
	for _, pkg := range packages {
		result = cons(vpkg(pkg), result)
	}
	return result, nil
}

func builtinRenamePackage(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("rename-package: need package and new-name")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return nil, fmt.Errorf("rename-package: package not found")
	}
	newName := strings.ToUpper(primaryValue(args[1]).str)

	// Collect nicknames from options
	var nicknames []string
	for i := 2; i < len(args); i++ {
		n := primaryValue(args[i]).str
		if len(n) > 0 {
			nicknames = append(nicknames, n)
		}
	}

	// Remove old name from packages map
	delete(packages, pkg.name)
	for _, n := range pkg.nicknames {
		delete(packages, n)
	}

	// Update package
	pkg.name = newName
	pkg.nicknames = nicknames
	packages[newName] = pkg
	for _, n := range nicknames {
		packages[strings.ToUpper(n)] = pkg
	}

	return vpkg(pkg), nil
}

func builtinDeletePackage(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("delete-package: need package designator")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return vbool(false), nil
	}
	// Remove from packages map
	delete(packages, pkg.name)
	for _, n := range pkg.nicknames {
		delete(packages, n)
	}
	// Can't delete if it has external symbols, but for simplicity just delete
	return vbool(true), nil
}

func builtinPackageNicknames(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("package-nicknames: need package designator")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return nil, fmt.Errorf("package-nicknames: package not found")
	}
	result := vnil()
	for i := len(pkg.nicknames) - 1; i >= 0; i-- {
		result = cons(vsym(pkg.nicknames[i]), result)
	}
	return result, nil
}

func builtinPackageSymbols(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("package-symbols: need package designator")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return vnil(), nil
	}
	result := vnil()
	for _, sym := range pkg.symbols {
		result = cons(sym, result)
	}
	return result, nil
}

func builtinPackageExternalSymbols(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("package-external-symbols: need package designator")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return vnil(), nil
	}
	result := vnil()
	for name, isExported := range pkg.exports {
		if isExported {
			if sym, ok := pkg.symbols[name]; ok {
				result = cons(sym, result)
			}
		}
	}
	return result, nil
}

func builtinUnexport(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("unexport: need symbols")
	}
	pkg := currentPackage
	syms := args
	if len(args) >= 2 {
		pkg2 := resolvePackageFromDesignator(primaryValue(args[0]))
		if pkg2 != nil {
			pkg = pkg2
			syms = args[1:]
		}
	}
	for _, s := range syms {
		symName := primaryValue(s).str
		delete(pkg.exports, symName)
	}
	return vbool(true), nil
}

func builtinPackageUseList(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("package-use-list: need package designator")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return vnil(), nil
	}
	result := vnil()
	for _, used := range pkg.used {
		result = cons(vpkg(used), result)
	}
	return result, nil
}

func builtinPackageUsedByList(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("package-used-by-list: need package designator")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return vnil(), nil
	}
	var usedBy []*Package
	for _, p := range packages {
		for _, u := range p.used {
			if u == pkg {
				usedBy = append(usedBy, p)
				break
			}
		}
	}
	result := vnil()
	for _, p := range usedBy {
		result = cons(vpkg(p), result)
	}
	return result, nil
}

func builtinPackageShadowingImportList(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("package-shadowing-import-list: need package designator")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return vnil(), nil
	}
	result := vnil()
	for _, symName := range pkg.shadowingImps {
		result = cons(vsym(symName), result)
	}
	return result, nil
}

func builtinProvide(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VSym {
		return nil, fmt.Errorf("provide: need a module name (symbol)")
	}
	name := args[0].str
	lispModules[name] = true

	// Update *modules* list in global env
	var list *Value
	for m := range lispModules {
		list = cons(vsym(m), list)
	}
	globalEnv.Set("*modules*", list)
	return vsym(name), nil
}

func builtinRequire(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VSym {
		return nil, fmt.Errorf("require: need a module name (symbol)")
	}
	name := args[0].str
	if lispModules[name] {
		return vsym(name), nil // already loaded
	}

	// Try to load <name>.lisp
	filename := name + ".lisp"
	_, err := LoadFile(filename, globalEnv)
	if err != nil {
		return nil, fmt.Errorf("require: cannot load %s: %v", filename, err)
	}

	// Mark as loaded (loadFile may have called provide, but be safe)
	if !lispModules[name] {
		lispModules[name] = true
		var list *Value
		for m := range lispModules {
			list = cons(vsym(m), list)
		}
		globalEnv.Set("*modules*", list)
	}
	return vsym(name), nil
}

func builtinImport(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VSym {
		return nil, fmt.Errorf("import: need symbol")
	}
	pkg := currentPackage
	if len(args) >= 2 {
		pkg2 := resolvePackageFromDesignator(primaryValue(args[1]))
		if pkg2 != nil {
			pkg = pkg2
		}
	}
	symName := primaryValue(args[0]).str
	pkg.symbols[symName] = args[0]
	return args[0], nil
}

func builtinUsePackage(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("use-package: need package name(s)")
	}
	pkgs := args[0]
	// Accept a list of package names or a single symbol
	if pkgs.typ == VPair {
		for !isNil(pkgs) {
			if err := useOnePackage(pkgs.car); err != nil {
				return nil, err
			}
			pkgs = pkgs.cdr
		}
		return vbool(true), nil
	}
	if err := useOnePackage(pkgs); err != nil {
		return nil, err
	}
	return vbool(true), nil
}

func useOnePackage(v *Value) error {
	srcPkg := resolvePackageFromDesignator(primaryValue(v))
	if srcPkg == nil {
		return fmt.Errorf("use-package: package not found")
	}
	pkg := currentPackage
	for symName, exported := range srcPkg.exports {
		if exported {
			if _, ok := pkg.symbols[symName]; !ok {
				if sym, ok := srcPkg.symbols[symName]; ok {
					pkg.symbols[symName] = sym
				}
			}
		}
	}
	pkg.used = append(pkg.used, srcPkg)
	return nil
}

func builtinUnusePackage(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("unuse-package: need package name(s)")
	}
	srcPkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if srcPkg == nil {
		return nil, fmt.Errorf("unuse-package: package not found")
	}
	pkg := currentPackage
	if len(args) >= 2 {
		pkg2 := resolvePackageFromDesignator(primaryValue(args[1]))
		if pkg2 != nil {
			pkg = pkg2
		}
	}
	// Remove srcPkg from used list
	newUsed := make([]*Package, 0, len(pkg.used))
	for _, p := range pkg.used {
		if p != srcPkg {
			newUsed = append(newUsed, p)
		}
	}
	pkg.used = newUsed
	return vbool(true), nil
}

func builtinShadow(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VSym {
		return nil, fmt.Errorf("shadow: need symbol")
	}
	pkg := currentPackage
	if len(args) >= 2 {
		pkg2 := resolvePackageFromDesignator(primaryValue(args[1]))
		if pkg2 != nil {
			pkg = pkg2
		}
	}
	symName := primaryValue(args[0]).str
	pkg.symbols[symName] = args[0]
	pkg.exports[symName] = true
	return args[0], nil
}

func builtinUnintern(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VSym {
		return nil, fmt.Errorf("unintern: need symbol")
	}
	pkg := currentPackage
	if len(args) >= 2 && args[1].typ == VSym {
		pkg = findPackage(args[1].str)
		if pkg == nil {
			return nil, fmt.Errorf("unintern: package not found")
		}
	}
	symName := args[0].str
	delete(pkg.symbols, symName)
	delete(pkg.exports, symName)
	return args[0], nil
}

func builtinShadowingImport(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("shadowing-import: need symbol or list")
	}
	pkg := currentPackage
	if len(args) >= 2 && args[len(args)-1].typ == VSym {
		lastArg := args[len(args)-1]
		if p := findPackage(lastArg.str); p != nil {
			pkg = p
			args = args[:len(args)-1]
		}
	}
	symbols := args
	if len(args) == 1 && args[0].typ == VPair {
		seen := make(map[*Value]bool)
		for cur := args[0]; !isNil(cur) && cur.typ == VPair; cur = cur.cdr {
			if seen[cur] {
				break
			}
			seen[cur] = true
			symbols = append(symbols, cur.car)
		}
		symbols = symbols[1:]
	}
	for _, sym := range symbols {
		if sym.typ != VSym {
			return nil, fmt.Errorf("shadowing-import: need symbol, got %s", typeStr(sym))
		}
		symName := sym.str
		pkg.symbols[symName] = sym
		pkg.exports[symName] = true
		// Track shadowing imports
		found := false
		for _, s := range pkg.shadowingImps {
			if s == symName {
				found = true
				break
			}
		}
		if !found {
			pkg.shadowingImps = append(pkg.shadowingImps, symName)
		}
	}
	return vsym("t"), nil
}

// -------- with-output-to-string (special form) --------
// Implemented as special form in eval
