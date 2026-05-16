package microlisp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// -------- Pathname helpers --------

func vpathname(p *LispPathname) *Value {
	v := gcv()
	v.typ = VPathname
	v.pathname = p
	return v
}

func vpkg(p *Package) *Value {
	v := gcv()
	v.typ = VPackage
	v.pkg = p
	return v
}

func isPackage(v *Value) bool {
	return v != nil && v.typ == VPackage
}

func builtinPackageP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(isPackage(args[0])), nil
}

func builtinCompiledFunctionP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]
	// In microlisp, all built-in and defined functions are "compiled"
	return vbool(v.typ == VPrim || v.typ == VFunc || v.typ == VGeneric), nil
}

func vrt(rt *Readtable) *Value {
	v := gcv()
	v.typ = VReadtable
	v.readtable = rt
	return v
}

func isReadtable(v *Value) bool {
	return v != nil && v.typ == VReadtable
}

func getPathname(v *Value) *LispPathname {
	if v.typ == VPathname {
		return v.pathname
	}
	if v.typ == VStr {
		return parsePathnameString(v.str)
	}
	return nil
}

func builtinParseNamestring(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("parse-namestring: need a namestring")
	}
	v := args[0]
	if v.typ == VPathname {
		// Already a pathname
		return multiVal(v, vnum(0)), nil
	}
	if v.typ == VStr {
		pn := parsePathnameString(v.str)
		return multiVal(vpathname(pn), vnum(float64(len(v.str)))), nil
	}
	return nil, fmt.Errorf("parse-namestring: not a valid namestring")
}

func parsePathnameString(s string) *LispPathname {
	p := &LispPathname{version: "newest"}
	// Handle logical pathname (e.g., "SYS:FOO;BAR.LISP")
	// A logical host has multiple alphabetic characters before a colon,
	// unlike a Windows drive letter which is a single character.
	if colonIdx := strings.Index(s, ":"); colonIdx > 1 {
		// Multi-char host: treat as logical pathname
		p.host = strings.ToUpper(s[:colonIdx])
		s = s[colonIdx+1:]
		// Logical pathnames use semicolons as directory separators
		if len(s) > 0 && s[0] == ';' {
			p.directory = list(vsym(":ABSOLUTE"))
			s = s[1:]
		} else {
			p.directory = list(vsym(":RELATIVE"))
		}
		// Split on semicolons (logical pathname directory separator)
		parts := []string{}
		start := 0
		for i := 0; i < len(s); i++ {
			if s[i] == ';' {
				if i > start {
					parts = append(parts, s[start:i])
				}
				start = i + 1
			}
		}
		// Last part may contain name.type
		last := s[start:]
		if len(parts) > 0 || len(last) > 0 {
			// Check if last part has a dot (extension separator)
			dotIdx := -1
			for j := len(last) - 1; j >= 0; j-- {
				if last[j] == '.' {
					dotIdx = j
					break
				}
			}
			if dotIdx > 0 {
				p.name = strings.ToUpper(last[:dotIdx])
				p.ftype = strings.ToUpper(last[dotIdx+1:])
			} else if dotIdx == 0 {
				p.ftype = strings.ToUpper(last[1:])
			} else {
				p.name = strings.ToUpper(last)
			}
		}
		// Build directory list
		dirList := p.directory
		for _, part := range parts {
			dirList = appendToList(dirList, vsym(strings.ToUpper(part)))
		}
		p.directory = dirList
		return p
	}
	// Handle Windows paths (e.g., C:/)
	if len(s) >= 2 && s[1] == ':' {
		p.device = s[:2]
		s = s[2:]
	}
	// Determine absolute vs relative
	if len(s) > 0 && (s[0] == '/' || s[0] == '\\') {
		p.directory = list(vsym(":ABSOLUTE"))
		// Skip leading separators
		for len(s) > 0 && (s[0] == '/' || s[0] == '\\') {
			s = s[1:]
		}
	} else {
		p.directory = list(vsym(":RELATIVE"))
	}
	// Split directory components
	parts := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '/' || s[i] == '\\' {
			if i > start {
				parts = append(parts, s[start:i])
			}
			start = i + 1
		}
	}
	// Last part may contain name.type
	last := s[start:]
	if len(parts) > 0 || len(last) > 0 {
		// Check if last part has a dot (extension separator)
		dotIdx := -1
		for j := len(last) - 1; j >= 0; j-- {
			if last[j] == '.' {
				dotIdx = j
				break
			}
		}
		if dotIdx > 0 {
			p.name = last[:dotIdx]
			p.ftype = last[dotIdx+1:]
		} else if dotIdx == 0 {
			p.ftype = last[1:]
		} else {
			p.name = last
		}
	}
	// Build directory list
	dirList := p.directory
	for _, part := range parts {
		dirList = appendToList(dirList, vsym(part))
	}
	p.directory = dirList
	return p
}

func pathnameToString(p *LispPathname) string {
	var b strings.Builder
	if p.device != "" {
		b.WriteString(p.device)
	}
	// Directory
	if p.directory != nil && !isNil(p.directory) {
		dir := p.directory
		if !isNil(dir) && dir.car != nil && dir.car.typ == VSym {
			if dir.car.str == ":ABSOLUTE" {
				b.WriteString("/")
			}
			dir = dir.cdr
		}
		for !isNil(dir) && dir.typ == VPair {
			if dir.car != nil && dir.car.typ == VSym {
				b.WriteString(dir.car.str)
				b.WriteString("/")
			} else if dir.car != nil && dir.car.typ == VStr {
				b.WriteString(dir.car.str)
				b.WriteString("/")
			}
			dir = dir.cdr
		}
	}
	// Name
	b.WriteString(p.name)
	// Type
	if p.ftype != "" {
		b.WriteString(".")
		b.WriteString(p.ftype)
	}
	return b.String()
}

func appendToList(lst *Value, elem *Value) *Value {
	if isNil(lst) {
		return cons(elem, vnil())
	}
	// Iterate to find last cons cell
	cur := lst
	for cur.typ == VPair && !isNil(cur.cdr) && cur.cdr.typ == VPair {
		cur = cur.cdr
	}
	cur.cdr = cons(elem, vnil())
	return lst
}

func builtinMakePathname(args []*Value) (*Value, error) {
	p := &LispPathname{version: "newest"}
	for i := 0; i+1 < len(args); i += 2 {
		key := args[i]
		val := args[i+1]
		if key.typ != VSym || len(key.str) == 0 || key.str[0] != ':' {
			continue
		}
		switch key.str[1:] {
		case "host":
			if val.typ == VStr {
				p.host = val.str
			}
		case "device":
			if val.typ == VStr {
				p.device = val.str
			}
		case "directory":
			p.directory = val
		case "name":
			if val.typ == VStr {
				p.name = val.str
			} else if val.typ == VSym {
				p.name = val.str
			}
		case "type":
			if val.typ == VStr {
				p.ftype = val.str
			} else if val.typ == VSym {
				p.ftype = val.str
			}
		case "version":
			if val.typ == VSym {
				p.version = val.str
			} else if val.typ == VStr {
				p.version = val.str
			}
		}
	}
	return vpathname(p), nil
}

func builtinPathname(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("pathname: need a pathname designator")
	}
	v := args[0]
	if v.typ == VPathname {
		return v, nil
	}
	if v.typ == VStr {
		return vpathname(parsePathnameString(v.str)), nil
	}
	if v.typ == VStream && v.stream != nil && v.stream.path != "" {
		return vpathname(parsePathnameString(v.stream.path)), nil
	}
	return nil, fmt.Errorf("pathname: cannot convert to pathname")
}

func builtinPathnameHost(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	p := getPathname(args[0])
	if p == nil || p.host == "" {
		return vnil(), nil
	}
	return vstr(p.host), nil
}

func builtinPathnameDevice(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	p := getPathname(args[0])
	if p == nil || p.device == "" {
		return vnil(), nil
	}
	return vstr(p.device), nil
}

func builtinPathnameDirectory(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	p := getPathname(args[0])
	if p == nil || p.directory == nil {
		return vnil(), nil
	}
	return p.directory, nil
}

func builtinPathnameName(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	p := getPathname(args[0])
	if p == nil || p.name == "" {
		return vnil(), nil
	}
	return vstr(p.name), nil
}

func builtinPathnameType(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	p := getPathname(args[0])
	if p == nil || p.ftype == "" {
		return vnil(), nil
	}
	return vstr(p.ftype), nil
}

func builtinPathnameVersion(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	p := getPathname(args[0])
	if p == nil {
		return vsym(":newest"), nil
	}
	return vsym(p.version), nil
}

func builtinNamestring(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vstr(""), nil
	}
	p := getPathname(args[0])
	if p == nil {
		return vstr(""), nil
	}
	return vstr(pathnameToString(p)), nil
}

func builtinFileNamestring(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vstr(""), nil
	}
	p := getPathname(args[0])
	if p == nil {
		return vstr(""), nil
	}
	var b strings.Builder
	b.WriteString(p.name)
	if p.ftype != "" {
		b.WriteString(".")
		b.WriteString(p.ftype)
	}
	return vstr(b.String()), nil
}

func builtinDirectoryNamestring(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vstr(""), nil
	}
	p := getPathname(args[0])
	if p == nil {
		return vstr(""), nil
	}
	var b strings.Builder
	if p.device != "" {
		b.WriteString(p.device)
	}
	if p.directory != nil && !isNil(p.directory) {
		dir := p.directory
		if !isNil(dir) && dir.car != nil && dir.car.typ == VSym {
			if dir.car.str == ":ABSOLUTE" {
				b.WriteString("/")
			}
			dir = dir.cdr
		}
		for !isNil(dir) && dir.typ == VPair {
			if dir.car != nil && dir.car.typ == VSym {
				b.WriteString(dir.car.str)
				b.WriteString("/")
			} else if dir.car != nil && dir.car.typ == VStr {
				b.WriteString(dir.car.str)
				b.WriteString("/")
			}
			dir = dir.cdr
		}
	}
	return vstr(b.String()), nil
}

func builtinHostNamestring(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vstr(""), nil
	}
	p := getPathname(args[0])
	if p == nil || p.host == "" {
		return vstr(""), nil
	}
	return vstr(p.host), nil
}

func builtinEnoughNamestring(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vstr(""), nil
	}
	p := getPathname(args[0])
	if p == nil {
		return vstr(""), nil
	}
	var b strings.Builder
	b.WriteString(p.name)
	if p.ftype != "" {
		b.WriteString(".")
		b.WriteString(p.ftype)
	}
	return vstr(b.String()), nil
}

func builtinMergePathnames(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("merge-pathnames: need a pathname")
	}
	p := getPathname(args[0])
	if p == nil {
		p = &LispPathname{version: "newest"}
	}
	var dp *LispPathname
	if len(args) >= 2 {
		dp = getPathname(args[1])
	}
	if dp == nil {
		dp = &LispPathname{version: "newest"}
	}
	result := &LispPathname{version: "newest"}
	result.host = p.host
	if result.host == "" {
		result.host = dp.host
	}
	result.device = p.device
	if result.device == "" {
		result.device = dp.device
	}
	result.directory = p.directory
	if result.directory == nil || isNil(result.directory) {
		result.directory = dp.directory
	}
	result.name = p.name
	if result.name == "" {
		result.name = dp.name
	}
	result.ftype = p.ftype
	if result.ftype == "" {
		result.ftype = dp.ftype
	}
	result.version = p.version
	if result.version == "" {
		result.version = dp.version
	}
	return vpathname(result), nil
}

func builtinUserHomedirPathname(args []*Value) (*Value, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/"
	}
	// Split home path into directory components
	parts := strings.Split(strings.TrimRight(home, "/\\"), string(os.PathSeparator))
	dirList := vnil()
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			dirList = cons(vstr(parts[i]), dirList)
		}
	}
	dirList = cons(vsym(":ABSOLUTE"), dirList)
	p := &LispPathname{
		directory: dirList,
		name:      "",
		ftype:     "",
	}
	return vpathname(p), nil
}

func builtinPathnamep(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VPathname), nil
}

func builtinProbeFile(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("probe-file: need a pathname")
	}
	p := getPathname(args[0])
	if p == nil {
		return vnil(), nil
	}
	path := pathnameToString(p)
	if _, err := os.Stat(path); err == nil {
		return vpathname(p), nil
	}
	return vnil(), nil
}

func builtinDirectory(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	p := getPathname(args[0])
	if p == nil {
		return vnil(), nil
	}
	path := pathnameToString(p)
	matches, err := filepath.Glob(path)
	if err != nil {
		return vnil(), nil
	}
	result := vnil()
	for i := len(matches) - 1; i >= 0; i-- {
		pp := parsePathnameString(matches[i])
		result = cons(vpathname(pp), result)
	}
	return result, nil
}

func builtinFileLength(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("file-length: need a stream")
	}
	v := args[0]
	if v.typ == VStream && v.stream != nil && v.stream.file != nil {
		fi, err := v.stream.file.Stat()
		if err != nil {
			return vnil(), nil
		}
		return vnum(float64(fi.Size())), nil
	}
	return vnil(), nil
}

func builtinFilePosition(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("file-position: need a stream")
	}
	v := args[0]
	if v.typ != VStream || v.stream == nil {
		return vnil(), nil
	}
	if len(args) < 2 {
		if v.stream.file != nil {
			pos, err := v.stream.file.Seek(0, 1)
			if err != nil {
				return vnil(), nil
			}
			return vnum(float64(pos)), nil
		}
		return vnil(), nil
	}
	pos := int64(toNum(args[1]))
	if v.stream.file != nil {
		_, err := v.stream.file.Seek(pos, 0)
		if err != nil {
			return vnil(), nil
		}
		return vnum(float64(pos)), nil
	}
	return vnil(), nil
}

func builtinTruename(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("truename: need a pathname")
	}
	p := getPathname(args[0])
	if p == nil {
		return vnil(), nil
	}
	path := pathnameToString(p)
	abs, err := filepath.Abs(path)
	if err != nil {
		return vnil(), nil
	}
	return vpathname(parsePathnameString(abs)), nil
}

// -------- Additional pathname functions --------

// directory-pathname-p returns true if pathname has no name/type/version
func builtinDirectoryPathnameP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	p := getPathname(args[0])
	if p == nil {
		return vbool(false), nil
	}
	return vbool(p.name == "" && p.ftype == "" && p.version == ""), nil
}

// wild-pathname-p checks if pathname contains wildcard components
func builtinWildPathnameP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	p := getPathname(args[0])
	if p == nil {
		return vbool(false), nil
	}
	// Check for :wild in components, or * in name/type
	if p.name == "*" || p.ftype == "*" || p.version == "*" {
		return vbool(true), nil
	}
	if p.directory != nil && !isNil(p.directory) {
		for d := p.directory; !isNil(d) && d.typ == VPair; d = d.cdr {
			if d.car != nil && d.car.typ == VSym && d.car.str == ":WILD" {
				return vbool(true), nil
			}
			if d.car != nil && d.car.typ == VStr && d.car.str == "*" {
				return vbool(true), nil
			}
		}
	}
	return vbool(false), nil
}

// pathname-match-p checks if pathname matches a wildcard pattern
func builtinPathnameMatchP(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	pathP := getPathname(args[0])
	patternP := getPathname(args[1])
	if pathP == nil || patternP == nil {
		return vbool(false), nil
	}
	// Simple matching: * matches anything, otherwise exact match
	if patternP.name == "*" || patternP.name == pathP.name {
		if patternP.ftype == "*" || patternP.ftype == pathP.ftype {
			return vbool(true), nil
		}
	}
	return vbool(false), nil
}

// file-author returns the author of a file
func builtinFileAuthor(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("file-author: need a pathname")
	}
	p := getPathname(args[0])
	if p == nil {
		return vnil(), nil
	}
	path := pathnameToString(p)
	_, err := os.Stat(path)
	if err != nil {
		return vnil(), nil
	}
	// Go doesn't natively support file author on all platforms; return nil
	return vnil(), nil
}

// file-write-date returns the modification time as universal time
func builtinFileWriteDate(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("file-write-date: need a pathname")
	}
	p := getPathname(args[0])
	if p == nil {
		return vnil(), nil
	}
	path := pathnameToString(p)
	info, err := os.Stat(path)
	if err != nil {
		return vnil(), nil
	}
	// Convert to Unix timestamp (Lisp universal time is seconds since 1900-01-01)
	modTime := info.ModTime().Unix()
	// Universal time epoch: 1900-01-01 00:00:00 UTC
	unixEpoch := int64(2208988800) // seconds from 1900 to 1970
	return vnum(float64(modTime + unixEpoch)), nil
}

// builtinRenameFile renames a file
func builtinRenameFile(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("rename-file: need old and new pathname")
	}
	oldP := getPathname(args[0])
	newP := getPathname(args[1])
	if oldP == nil || newP == nil {
		return vnil(), nil
	}
	oldPath := pathnameToString(oldP)
	newPath := pathnameToString(newP)
	err := os.Rename(oldPath, newPath)
	if err != nil {
		return vnil(), nil
	}
	oldAbs, _ := filepath.Abs(oldPath)
	newAbs, _ := filepath.Abs(newPath)
	return list(vpathname(parsePathnameString(newAbs)), vpathname(parsePathnameString(newAbs)), vpathname(parsePathnameString(oldAbs))), nil
}

// builtinDeleteFile deletes a file
func builtinDeleteFile(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("delete-file: need a pathname")
	}
	p := getPathname(args[0])
	if p == nil {
		return vbool(false), nil
	}
	path := pathnameToString(p)
	err := os.Remove(path)
	if err != nil {
		return vbool(false), nil
	}
	return vbool(true), nil
}

// ensure-directories-exist ensures parent directories exist for a pathname
func builtinEnsureDirectoriesExist(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("ensure-directories-exist: need a pathname")
	}
	p := getPathname(args[0])
	if p == nil {
		return vnil(), nil
	}
	path := pathnameToString(p)
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return vbool(false), nil
	}
	return vpathname(p), nil
}

// logical-pathname translates a logical pathname to physical
func builtinLogicalPathname(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("logical-pathname: need a pathname")
	}
	v := args[0]
	var pathStr string
	if v.typ == VStr {
		pathStr = v.str
	} else if v.typ == VPathname {
		pathStr = pathnameToString(v.pathname)
	}
	// Treat as physical and return as-is for now
	return vpathname(parsePathnameString(pathStr)), nil
}

func builtinTranslatePathname(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("translate-pathname: need source from-wildcard to-wildcard")
	}
	source := getPathname(args[0])
	fromP := getPathname(args[1])
	toP := getPathname(args[2])
	if source == nil || fromP == nil || toP == nil {
		return nil, fmt.Errorf("translate-pathname: all arguments must be pathnames")
	}
	// Translate each component: substitute from-wildcard matches into to-wildcard template
	result := &LispPathname{version: "newest"}
	result.host = translateComponent(source.host, fromP.host, toP.host)
	result.device = translateComponent(source.device, fromP.device, toP.device)
	result.name = translateComponent(source.name, fromP.name, toP.name)
	result.ftype = translateComponent(source.ftype, fromP.ftype, toP.ftype)
	// For directory, just copy from source if from matches
	result.directory = source.directory
	return vpathname(result), nil
}

func translateComponent(source, from, to string) string {
	if from == "*" || from == "" {
		if to == "*" {
			return source
		}
		return to
	}
	if source == from {
		return to
	}
	// Simple wildcard matching: if from is "*", replace with source in to
	if strings.Contains(from, "*") && to != "" {
		// Replace first wildcard in 'to' with the matched portion of source
		parts := strings.SplitN(from, "*", 2)
		if strings.HasPrefix(source, parts[0]) {
			matched := strings.TrimPrefix(source, parts[0])
			if len(parts) > 1 && parts[1] != "" {
				matched = strings.TrimSuffix(matched, parts[1])
			}
			return strings.Replace(to, "*", matched, 1)
		}
	}
	if to == "" {
		return source
	}
	return to
}

func builtinTranslateLogicalPathname(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("translate-logical-pathname: need a pathname")
	}
	p := getPathname(args[0])
	if p == nil {
		return nil, fmt.Errorf("translate-logical-pathname: need a pathname")
	}
	// If the pathname has a logical host, look up translations
	if p.host != "" {
		translations := logicalPathnameTranslations[strings.ToUpper(p.host)]
		if translations != nil && !isNil(translations) {
			// Apply each translation rule
			cur := translations
			for !isNil(cur) && cur.typ == VPair {
				rule := cur.car
				if rule.typ == VPair && !isNil(rule) {
					fromP := getPathname(rule.car)
					toP := getPathname(rule.cdr.car)
					if fromP != nil && toP != nil {
						// Check if source matches from pattern
						if pathnameMatchesPattern(p, fromP) {
							return builtinTranslatePathname([]*Value{vpathname(p), vpathname(fromP), vpathname(toP)})
						}
					}
				}
				cur = cur.cdr
			}
		}
	}
	// Not a logical pathname or no translation found; return as-is
	return vpathname(p), nil
}

func pathnameMatchesPattern(source, pattern *LispPathname) bool {
	if pattern.host != "" && pattern.host != "*" && pattern.host != source.host {
		return false
	}
	if pattern.name != "" && pattern.name != "*" && pattern.name != source.name {
		return false
	}
	if pattern.ftype != "" && pattern.ftype != "*" && pattern.ftype != source.ftype {
		return false
	}
	return true
}

func builtinLogicalPathnameTranslations(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("logical-pathname-translations: need a host")
	}
	host := strings.ToUpper(ToString(args[0]))
	if len(args) >= 2 {
		// SETF form: set the translations
		logicalPathnameTranslations[host] = args[1]
		return args[1], nil
	}
	// Get the translations
	t, ok := logicalPathnameTranslations[host]
	if !ok {
		return vnil(), nil
	}
	return t, nil
}

func builtinLogicalPathnameTranslationsSetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("logical-pathname-translations-setf: need new-value and host")
	}
	host := strings.ToUpper(ToString(args[1]))
	logicalPathnameTranslations[host] = args[0]
	return args[0], nil
}
