package microlisp

import (
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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

func toNum(v *Value) float64 {
	switch v.typ {
	case VNum:
		return v.num
	case VRat:
		return float64(v.irat) / float64(v.iden)
	case VComplex:
		return v.num
	case VBigInt:
		f, _ := new(big.Float).SetInt(v.bigInt).Float64()
		return f
	}
	return 0
}

// isNumeric returns true if v is a numeric type
func isNumeric(v *Value) bool {
	return v.typ == VNum || v.typ == VRat || v.typ == VComplex || v.typ == VBigInt
}

// toRatParts extracts rational form. isInt indicates VNum with integer value.
func toRatParts(v *Value) (n, d int64, isInt bool) {
	switch v.typ {
	case VRat:
		return v.irat, v.iden, true
	case VNum:
		if v.num == math.Trunc(v.num) && !math.IsInf(v.num, 0) && v.num >= -9e15 && v.num <= 9e15 {
			return int64(v.num), 1, true
		}
	case VBigInt:
		if v.bigInt.IsInt64() {
			return v.bigInt.Int64(), 1, true
		}
	}
	return 0, 0, false
}

// toComplexParts extracts real and imaginary parts from any numeric value.
func toComplexParts(v *Value) (r, i float64) {
	switch v.typ {
	case VComplex:
		return v.num, v.imag
	case VRat:
		return float64(v.irat) / float64(v.iden), 0
	default:
		return toNum(v), 0
	}
}

// needComplex checks if any arg is a complex number.
func needComplex(args []*Value) bool {
	for _, a := range args {
		if a.typ == VComplex {
			return true
		}
	}
	return false
}

// needRat checks if any arg is a rational (and none are complex).
func needRat(args []*Value) bool {
	for _, a := range args {
		if a.typ == VRat || a.typ == VBigInt {
			return true
		}
	}
	return false
}

// isBigIntInt checks if any arg is a VBigInt.
func isBigIntInt(args []*Value) bool {
	for _, a := range args {
		if a.typ == VBigInt {
			return true
		}
	}
	return false
}

// toBigInt converts a Value to *big.Int (0 if not integer).
func toBigInt(v *Value) *big.Int {
	switch v.typ {
	case VNum:
		if v.num == math.Trunc(v.num) && !math.IsInf(v.num, 0) {
			return big.NewInt(int64(v.num))
		}
	case VRat:
		if v.iden == 1 {
			return big.NewInt(v.irat)
		}
	case VBigInt:
		return new(big.Int).Set(v.bigInt)
	}
	return nil
}

// toBigIntExact converts a Value to *big.Int if it is an exact integer.
// Returns nil for non-integer types (float, rational, complex).
func toBigIntExact(v *Value) *big.Int {
	switch v.typ {
	case VNum:
		if !v.isFloat && v.num == math.Trunc(v.num) && !math.IsInf(v.num, 0) {
			return big.NewInt(int64(v.num))
		}
	case VRat:
		if v.iden == 1 {
			return big.NewInt(v.irat)
		}
	case VBigInt:
		return new(big.Int).Set(v.bigInt)
	}
	return nil
}

// toBigRat converts a Value to big.Rat for exact rational comparison.
func toBigRat(v *Value) big.Rat {
	switch v.typ {
	case VNum:
		if v.isFloat {
			return *new(big.Rat).SetFloat64(v.num)
		}
		return *big.NewRat(int64(v.num), 1)
	case VRat:
		return *big.NewRat(v.irat, v.iden)
	case VBigInt:
		r := new(big.Rat).SetInt(v.bigInt)
		return *r
	}
	return *big.NewRat(0, 1)
}

// compareNumeric returns -1 if a < b, 0 if a == b, 1 if a > b.
func compareNumeric(a, b *Value) int {
	// Handle complex numbers specially - need to compare both parts
	if a.typ == VComplex || b.typ == VComplex {
		aReal, aImag := toComplexParts(a)
		bReal, bImag := toComplexParts(b)
		if aReal < bReal {
			return -1
		}
		if aReal > bReal {
			return 1
		}
		if aImag < bImag {
			return -1
		}
		if aImag > bImag {
			return 1
		}
		return 0
	}
	// Use big.Int for exact comparison when either operand is VBigInt
	aBi := toBigIntExact(a)
	bBi := toBigIntExact(b)
	if aBi != nil && bBi != nil {
		return aBi.Cmp(bBi)
	}
	// Use big.Rat for exact comparison when either operand is VRat
	if a.typ == VRat || b.typ == VRat {
		aRat := toBigRat(a)
		bRat := toBigRat(b)
		return aRat.Cmp(&bRat)
	}
	// Fall back to float64 comparison
	aReal, _ := toComplexParts(a)
	bReal, _ := toComplexParts(b)
	if aReal < bReal {
		return -1
	}
	if aReal > bReal {
		return 1
	}
	return 0
}

func builtinAdd(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(0), nil
	}
	// Type check: all args must be numeric
	for _, a := range args {
		if !isNumeric(a) {
			return nil, fmt.Errorf("+: not a number: %s", ToString(a))
		}
	}
	if needComplex(args) {
		r, i := 0.0, 0.0
		for _, a := range args {
			ar, ai := toComplexParts(a)
			r += ar
			i += ai
		}
		return vcomplex(r, i), nil
	}
	// Try big.Int if any arg is VBigInt or if rational arithmetic might overflow
	if isBigIntInt(args) {
		result := new(big.Int)
		for _, a := range args {
			bi := toBigInt(a)
			if bi != nil {
				result.Add(result, bi)
				continue
			}
			// Not an exact integer - fall back to float
			f, _ := new(big.Float).SetInt(result).Float64()
			for _, a2 := range args {
				f += toNum(a2)
			}
			return vfloat(f), nil
		}
		return vbigint(result), nil
	}
	if needRat(args) {
		// Track as rational
		n, d := int64(0), int64(1)
		hasFloat := false
		for _, a := range args {
			an, ad, isInt := toRatParts(a)
			if !isInt {
				hasFloat = true
				break
			}
			n = n*ad + an*d
			d = d * ad
			g := gcd(n, d)
			if g < 0 {
				g = -g
			}
			n /= g
			d /= g
		}
		if hasFloat {
			r := 0.0
			for _, a := range args {
				r += toNum(a)
			}
			return vfloat(r), nil
		}
		return vrat(n, d), nil
	}
	r := 0.0
	for _, a := range args {
		r += toNum(a)
	}
	return numOrFloat(r, args), nil
}

func builtinSub(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(0), nil
	}
	for _, a := range args {
		if !isNumeric(a) {
			return nil, fmt.Errorf("-: not a number: %s", ToString(a))
		}
	}
	if needComplex(args) {
		ar, ai := toComplexParts(args[0])
		if len(args) == 1 {
			return vcomplex(-ar, -ai), nil
		}
		for _, a := range args[1:] {
			br, bi := toComplexParts(a)
			ar -= br
			ai -= bi
		}
		return vcomplex(ar, ai), nil
	}
	if isBigIntInt(args) {
		result := toBigInt(args[0])
		if result == nil {
			f := toNum(args[0])
			for _, a := range args[1:] {
				f -= toNum(a)
			}
			return vfloat(f), nil
		}
		for _, a := range args[1:] {
			bi := toBigInt(a)
			if bi != nil {
				result.Sub(result, bi)
			} else {
				f, _ := new(big.Float).SetInt(result).Float64()
				for _, a2 := range args {
					f -= toNum(a2)
				}
				return vfloat(f), nil
			}
		}
		return vbigint(result), nil
	}
	if len(args) == 1 {
		if args[0].typ == VRat {
			return vrat(-args[0].irat, args[0].iden), nil
		}
		return vnum(-toNum(args[0])), nil
	}
	if needRat(args) {
		n0, d0, isInt0 := toRatParts(args[0])
		hasFloat := !isInt0
		for _, a := range args[1:] {
			an, ad, isInt := toRatParts(a)
			if !isInt {
				hasFloat = true
				break
			}
			n0 = n0*ad - an*d0
			d0 = d0 * ad
			g := gcd(n0, d0)
			if g < 0 {
				g = -g
			}
			n0 /= g
			d0 /= g
		}
		if hasFloat {
			r := toNum(args[0])
			for _, a := range args[1:] {
				r -= toNum(a)
			}
			return vfloat(r), nil
		}
		return vrat(n0, d0), nil
	}
	r := toNum(args[0])
	for _, a := range args[1:] {
		r -= toNum(a)
	}
	return numOrFloat(r, args), nil
}

func builtinMul(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(1), nil
	}
	for _, a := range args {
		if !isNumeric(a) {
			return nil, fmt.Errorf("*: not a number: %s", ToString(a))
		}
	}
	if needComplex(args) {
		r, i := 1.0, 0.0 // start with 1+0i
		for _, a := range args {
			ar, ai := toComplexParts(a)
			// (r + i*i) * (ar + ai*i) = (r*ar - i*ai) + (r*ai + i*ar)*i
			newR := r*ar - i*ai
			newI := r*ai + i*ar
			r, i = newR, newI
		}
		return vcomplex(r, i), nil
	}
	if isBigIntInt(args) {
		result := big.NewInt(1)
		for _, a := range args {
			bi := toBigInt(a)
			if bi != nil {
				result.Mul(result, bi)
				continue
			}
			f, _ := new(big.Float).SetInt(result).Float64()
			for _, a2 := range args {
				f *= toNum(a2)
			}
			return vfloat(f), nil
		}
		return vbigint(result), nil
	}
	if needRat(args) {
		n, d := int64(1), int64(1)
		hasFloat := false
		for _, a := range args {
			an, ad, isInt := toRatParts(a)
			if !isInt {
				hasFloat = true
				break
			}
			n *= an
			d *= ad
			g := gcd(n, d)
			if g < 0 {
				g = -g
			}
			n /= g
			d /= g
		}
		if hasFloat {
			r := 1.0
			for _, a := range args {
				r *= toNum(a)
			}
			return vfloat(r), nil
		}
		return vrat(n, d), nil
	}
	// All args are VNum — check if they are all integer-valued
	// If so, use big.Int to avoid overflow
	allInt := true
	for _, a := range args {
		if a.typ != VNum || a.num != math.Trunc(a.num) || math.IsInf(a.num, 0) {
			allInt = false
			break
		}
	}
	if allInt {
		result := big.NewInt(1)
		for _, a := range args {
			bi := big.NewInt(int64(a.num))
			result.Mul(result, bi)
		}
		return vbigint(result), nil
	}
	r := 1.0
	for _, a := range args {
		r *= toNum(a)
	}
	return numOrFloat(r, args), nil
}

func builtinDiv(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(1), nil
	}
	for _, a := range args {
		if !isNumeric(a) {
			return nil, fmt.Errorf("/: not a number: %s", ToString(a))
		}
	}
	if needComplex(args) {
		ar, ai := toComplexParts(args[0])
		if len(args) == 1 {
			// 1 / (ar + ai*i) = ar/(ar²+ai²) - ai/(ar²+ai²)*i
			den := ar*ar + ai*ai
			if den == 0 {
				return nil, signalDivisionByZero()
			}
			return vcomplex(ar/den, -ai/den), nil
		}
		for _, a := range args[1:] {
			br, bi := toComplexParts(a)
			den := br*br + bi*bi
			if den == 0 {
				return nil, signalDivisionByZero()
			}
			// (ar + ai*i) / (br + bi*i) = (ar*br + ai*bi)/den + (ai*br - ar*bi)/den * i
			newR := (ar*br + ai*bi) / den
			newI := (ai*br - ar*bi) / den
			ar, ai = newR, newI
		}
		return vcomplex(ar, ai), nil
	}
	if len(args) == 1 {
		if args[0].typ == VBigInt {
			if args[0].bigInt.Sign() == 0 {
				return nil, signalDivisionByZero()
			}
			return vnum(1.0 / toNum(args[0])), nil
		}
		if args[0].typ == VRat {
			if args[0].irat == 0 {
				return nil, signalDivisionByZero()
			}
			// 1 / (a/b) = b/a
			n := args[0].iden
			d := args[0].irat
			if d < 0 {
				n = -n
				d = -d
			}
			return vrat(n, d), nil
		}
		if toNum(args[0]) == 0 {
			return nil, signalDivisionByZero()
		}
		return vnum(1.0 / toNum(args[0])), nil
	}
	if isBigIntInt(args) {
		num := toBigInt(args[0])
		den := big.NewInt(1)
		if num == nil {
			r := toNum(args[0])
			for _, a := range args[1:] {
				if toNum(a) == 0 {
					return nil, signalDivisionByZero()
				}
				r /= toNum(a)
			}
			return vfloat(r), nil
		}
		for _, a := range args[1:] {
			bi := toBigInt(a)
			if bi == nil {
				r := toNum(args[0])
				for _, a2 := range args[1:] {
					if toNum(a2) == 0 {
						return nil, signalDivisionByZero()
					}
					r /= toNum(a2)
				}
				return vfloat(r), nil
			}
			if bi.Sign() == 0 {
				return nil, signalDivisionByZero()
			}
			den.Mul(den, bi)
		}
		g := new(big.Int).GCD(nil, nil, num, den)
		if g.Sign() != 0 {
			num.Quo(num, g)
			den.Quo(den, g)
		}
		if den.Sign() < 0 {
			num.Neg(num)
			den.Neg(den)
		}
		if den.IsInt64() && den.Int64() == 1 {
			return vbigint(num), nil
		}
		// Result is not an integer: try to reduce to int64 rational
		if num.IsInt64() && den.IsInt64() {
			return vrat(num.Int64(), den.Int64()), nil
		}
		// Fallback: return as float
		f, _ := new(big.Float).Quo(
			new(big.Float).SetInt(num),
			new(big.Float).SetInt(den),
		).Float64()
		return vfloat(f), nil
	}
	if needRat(args) {
		n0, d0, isInt0 := toRatParts(args[0])
		hasFloat := !isInt0
		for _, a := range args[1:] {
			an, ad, isInt := toRatParts(a)
			if !isInt {
				hasFloat = true
				break
			}
			if an == 0 {
				return nil, signalDivisionByZero()
			}
			// (n0/d0) / (an/ad) = n0*ad / (d0*an)
			n0 *= ad
			d0 *= an
			if d0 < 0 {
				n0 = -n0
				d0 = -d0
			}
			g := gcd(n0, d0)
			if g < 0 {
				g = -g
			}
			n0 /= g
			d0 /= g
		}
		if hasFloat {
			r := toNum(args[0])
			for _, a := range args[1:] {
				if toNum(a) == 0 {
					return nil, signalDivisionByZero()
				}
				r /= toNum(a)
			}
			return vfloat(r), nil
		}
		return vrat(n0, d0), nil
	}
	// If all args are integers (not floats), use rational division
	allInt := true
	for _, a := range args {
		if a.typ == VNum && a.isFloat {
			allInt = false
			break
		}
	}
	if allInt {
		n0, d0 := int64(toNum(args[0])), int64(1)
		for _, a := range args[1:] {
			an := int64(toNum(a))
			if an == 0 {
				return nil, signalDivisionByZero()
			}
			n0 *= an
			d0 *= 1
		}
		// Simplify: n0/d0 where d0 is the product of all denominators
		// Actually we need n0 / (product of all args[1:])
		// Redo: numerator = args[0], denominator = product of args[1:]
		num := int64(toNum(args[0]))
		den := int64(1)
		for _, a := range args[1:] {
			den *= int64(toNum(a))
		}
		if den == 0 {
			return nil, signalDivisionByZero()
		}
		return vrat(num, den), nil
	}

	r := toNum(args[0])
	for _, a := range args[1:] {
		if toNum(a) == 0 {
			return nil, signalDivisionByZero()
		}
		r /= toNum(a)
	}
	return numOrFloat(r, args), nil
}

func builtinEq(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vbool(true), nil
	}
	for _, a := range args {
		if !isNumber(a) {
			return signalTypeError(a)
		}
	}
	if len(args) == 1 {
		return vbool(true), nil
	}
	for i := 1; i < len(args); i++ {
		if compareNumeric(args[i-1], args[i]) != 0 {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinNe(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	for i := 0; i < len(args); i++ {
		for j := i + 1; j < len(args); j++ {
			if compareNumeric(args[i], args[j]) == 0 {
				return vbool(false), nil
			}
		}
	}
	return vbool(true), nil
}

func builtinLt(args []*Value) (*Value, error) {
	for _, a := range args {
		if !isReal(a) {
			return signalTypeError(a)
		}
	}
	if len(args) < 2 {
		return vbool(true), nil
	}
	for i := 1; i < len(args); i++ {
		if compareNumeric(args[i-1], args[i]) >= 0 {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinGt(args []*Value) (*Value, error) {
	for _, a := range args {
		if !isReal(a) {
			return signalTypeError(a)
		}
	}
	if len(args) < 2 {
		return vbool(true), nil
	}
	for i := 1; i < len(args); i++ {
		if compareNumeric(args[i-1], args[i]) <= 0 {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinLe(args []*Value) (*Value, error) {
	for _, a := range args {
		if !isReal(a) {
			return signalTypeError(a)
		}
	}
	if len(args) < 2 {
		return vbool(true), nil
	}
	for i := 1; i < len(args); i++ {
		if compareNumeric(args[i-1], args[i]) > 0 {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinGe(args []*Value) (*Value, error) {
	for _, a := range args {
		if !isReal(a) {
			return signalTypeError(a)
		}
	}
	if len(args) < 2 {
		return vbool(true), nil
	}
	for i := 1; i < len(args); i++ {
		if compareNumeric(args[i-1], args[i]) < 0 {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinCons(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("cons: need 2 arguments")
	}
	return cons(args[0], args[1]), nil
}

func builtinCar(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("car: need 1 argument")
	}
	v := args[0]
	if v != nil && v.typ == VMultiVal {
		return primaryValue(v), nil
	}
	if isNil(v) {
		return vnil(), nil
	}
	if !isPair(v) {
		return nil, fmt.Errorf("car: not a pair")
	}
	return v.car, nil
}

func builtinCdr(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("cdr: need 1 argument")
	}
	v := args[0]
	if v != nil && v.typ == VMultiVal {
		v = primaryValue(v)
	}
	if isNil(v) {
		return vnil(), nil
	}
	if !isPair(v) {
		return nil, fmt.Errorf("cdr: not a pair")
	}
	return v.cdr, nil
}

func builtinSetCar(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-car!: need pair and value")
	}
	if !isPair(args[0]) {
		return nil, fmt.Errorf("set-car!: not a pair")
	}
	args[0].car = args[1]
	return args[1], nil
}

// builtinSet implements CL's (set symbol value) - sets the dynamic value of a symbol
func builtinSet(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set: need symbol and value")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("set: first argument must be a symbol")
	}
	val := args[1]
	globalEnv.Set(sym.str, val)
	return val, nil
}

func builtinSetCdr(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-cdr!: need pair and value")
	}
	if !isPair(args[0]) {
		return nil, fmt.Errorf("set-cdr!: not a pair")
	}
	args[0].cdr = args[1]
	return args[1], nil
}

// builtinSetCar is used for (setf (car x) val) -> (car-setf val x)
func builtinSetCarAsSetter(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("setf (car): need 2 arguments")
	}
	val := args[0]
	cons := args[1]
	if !isPair(cons) {
		return nil, fmt.Errorf("setf (car): not a pair")
	}
	cons.car = val
	return val, nil
}

// builtinSetCdrAsSetter is used for (setf (cdr x) val) -> (cdr-setf val x)
func builtinSetCdrAsSetter(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("setf (cdr): need 2 arguments")
	}
	val := args[0]
	cons := args[1]
	if !isPair(cons) {
		return nil, fmt.Errorf("setf (cdr): not a pair")
	}
	cons.cdr = val
	return val, nil
}

func builtinList(args []*Value) (*Value, error) {
	return listFromSlice(args), nil
}

// -------- FFI --------
var ffiRegistry = map[string]interface{}{
	"math/sin":   math.Sin,
	"math/cos":   math.Cos,
	"math/tan":   math.Tan,
	"math/sqrt":  math.Sqrt,
	"math/abs":   math.Abs,
	"math/floor": math.Floor,
	"math/ceil":  math.Ceil,
	"math/round": math.Round,
	"math/exp":   math.Exp,
	"math/log":   math.Log,
	"math/pow":   math.Pow,
	"os/getenv":  os.Getenv,
	"os/getpid":  os.Getpid,
}

func builtinFFI(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("ffi: need string function name")
	}
	name := args[0].str
	fn, ok := ffiRegistry[name]
	if !ok {
		return nil, fmt.Errorf("ffi: unknown function: %s", name)
	}
	fnVal := reflect.ValueOf(fn)
	// Handle non-function registered values
	if fnVal.Kind() != reflect.Func {
		return reflectToLisp(fnVal), nil
	}
	fnType := fnVal.Type()
	numIn := fnType.NumIn()
	// if variadic...
	callArgs := make([]reflect.Value, 0, len(args)-1)
	for i := 1; i < len(args); i++ {
		callArgs = append(callArgs, lispToReflect(args[i], fnType.In(min(i-1, numIn-1))))
	}
	// Handle variadic
	if fnType.IsVariadic() {
		fixedArgs := callArgs
		if len(fixedArgs) > numIn-1 {
			fixedArgs = callArgs[:numIn-1]
			varArgs := callArgs[numIn-1:]
			fixedArgs = append(fixedArgs, varArgs...)
			callArgs = fixedArgs
		}
	}
	results := fnVal.Call(callArgs)
	if len(results) == 0 {
		return vnil(), nil
	}
	return reflectToLisp(results[0]), nil
}

func builtinFFIRegister(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VStr {
		return nil, fmt.Errorf("ffi-register: need string name and value")
	}
	name := args[0].str
	// We can only register basic types here from Lisp
	// For Go functions, use the Go side
	if args[1].typ == VNum {
		ffiRegistry[name] = float64(toNum(args[1]))
	} else if args[1].typ == VStr {
		ffiRegistry[name] = args[1].str
	} else {
		return nil, fmt.Errorf("ffi-register: unsupported value type")
	}
	return vsym(name), nil
}

func builtinMacroexpand(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("macroexpand: need form")
	}
	form := args[0]
	depth := 0
	const maxMacroExpandDepth = 1000

	// Handle quasiquote specially since it's not a VMacro
	if form.typ == VPair && form.car != nil && form.car.typ == VSym && strings.EqualFold(form.car.str, "QUASIQUOTE") {
		if len(args) >= 1 && args[0].cdr != nil && args[0].cdr.typ == VPair && args[0].cdr.car != nil {
			expanded, e := evalQuasiquote(args[0].cdr.car, 0, globalEnv)
			if e != nil {
				return nil, fmt.Errorf("macroexpand: %s", e)
			}
			return list(vsym("quote"), expanded), nil
		}
		return form, nil
	}

	for form.typ == VPair && form.car != nil && form.car.typ == VSym {
		fn, err := globalEnv.Get(form.car.str)
		if err != nil || fn.typ != VMacro {
			break
		}
		depth++
		if depth > maxMacroExpandDepth {
			return nil, fmt.Errorf("macroexpand: expansion depth exceeded (%d)", maxMacroExpandDepth)
		}
		expanded, err := expandMacro(fn, form.cdr, globalEnv)
		if err != nil {
			return nil, fmt.Errorf("macroexpand: %s", err)
		}
		form = expanded
	}
	return form, nil
}

func builtinMacroexpand1(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("macroexpand-1: need form")
	}
	form := args[0]
	if form.typ == VPair && form.car != nil && form.car.typ == VSym {
		fn, err := globalEnv.Get(form.car.str)
		if err == nil && fn.typ == VMacro {
			expanded, err := expandMacro(fn, form.cdr, globalEnv)
			if err != nil {
				return nil, fmt.Errorf("macroexpand-1: %s", err)
			}
			return expanded, nil
		}
	}
	return form, nil
}

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

func builtinReadtableP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(isReadtable(args[0])), nil
}

func builtinMakeReadtable(args []*Value) (*Value, error) {
	rt := &Readtable{
		macroFns: make(map[rune]*macroEntry),
		dispFns:  make(map[rune]*macroEntry),
		caseMode: ":UPCASE",
	}
	return vrt(rt), nil
}

func builtinCopyReadtable(args []*Value) (*Value, error) {
	src := currentReadtable
	if len(args) >= 1 && isReadtable(args[0]) {
		src = args[0].readtable
	}
	rt := &Readtable{
		macroFns: make(map[rune]*macroEntry),
		dispFns:  make(map[rune]*macroEntry),
		caseMode: src.caseMode,
	}
	for k, v := range src.macroFns {
		entry := *v
		rt.macroFns[k] = &entry
	}
	for k, v := range src.dispFns {
		entry := *v
		rt.dispFns[k] = &entry
	}
	return vrt(rt), nil
}

func builtinReadtableCase(args []*Value) (*Value, error) {
	if len(args) < 1 || !isReadtable(args[0]) {
		return nil, fmt.Errorf("readtable-case: expected a readtable")
	}
	return mkKeyword(args[0].readtable.caseMode), nil
}

func builtinSetReadtableCase(args []*Value) (*Value, error) {
	if len(args) < 2 || !isReadtable(args[0]) {
		return nil, fmt.Errorf("set-readtable-case: expected readtable and case mode")
	}
	mode := args[1]
	var modeStr string
	if mode.typ == VSym && isKeyword(mode.str) {
		modeStr = mode.str
	} else if mode.typ == VStr {
		modeStr = ":" + strings.ToUpper(mode.str)
	} else {
		return nil, fmt.Errorf("set-readtable-case: invalid case mode %v", mode)
	}
	switch modeStr {
	case ":UPCASE", ":DOWNCASE", ":PRESERVE", ":INVERT":
		args[0].readtable.caseMode = modeStr
	default:
		return nil, fmt.Errorf("set-readtable-case: invalid case mode %s (expected :UPCASE, :DOWNCASE, :PRESERVE, or :INVERT)", modeStr)
	}
	return mkKeyword(modeStr), nil
}

func builtinSetMacroCharacter(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-macro-character: need char and function")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else {
		return nil, fmt.Errorf("set-macro-character: first argument must be a character")
	}
	fn := args[1]
	// CL spec: nil means remove the macro character
	if fn.typ == VNil {
		delete(currentReadtable.macroFns, ch)
		return vbool(true), nil
	}
	if fn.typ != VFunc && fn.typ != VPrim {
		return nil, fmt.Errorf("set-macro-character: second argument must be a function")
	}
	nonTerm := false
	if len(args) >= 3 {
		nonTerm = isTruthy(args[2])
	}
	rt := currentReadtable
	if len(args) >= 4 && isReadtable(args[3]) {
		rt = args[3].readtable
	}
	// Recognize close-paren sentinel: register as terminating macro character
	// (overrides the non-terminating flag if passed)
	isCloseParen := (fn == closeParenSentinel)
	rt.macroFns[ch] = &macroEntry{
		lispFn:      fn,
		terminating: isCloseParen || !nonTerm,
	}
	return vbool(true), nil
}

func builtinGetMacroCharacter(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("get-macro-character: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else {
		return nil, fmt.Errorf("get-macro-character: argument must be a character")
	}
	rt := currentReadtable
	if len(args) >= 2 && isReadtable(args[1]) {
		rt = args[1].readtable
	}
	entry, ok := rt.macroFns[ch]
	if !ok || entry == nil {
		return vnil(), nil
	}
	if entry.goFn != nil {
		// Return nil for Go-level macro functions (not introspectable as Lisp fns)
		return vnil(), nil
	}
	if entry.lispFn != nil {
		return entry.lispFn, nil
	}
	// Standard macro character with no Lisp-level or Go-level function.
	// For close-paren ')', return the sentinel so set-macro-character can
	// recognize it and register the target character as close-paren equivalent.
	if ch == ')' {
		return closeParenSentinel, nil
	}
	// For other standard macro chars, return a wrapper that signals an error
	// when called as a reader macro (since the real handling is in the lexer).
	return &Value{typ: VPrim, fn: func(args []*Value) (*Value, error) {
		return nil, fmt.Errorf("standard macro character %q cannot be invoked as a reader macro function", string(ch))
	}}, nil
}

func builtinMakeDispatchMacroCharacter(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-dispatch-macro-character: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else {
		return nil, fmt.Errorf("make-dispatch-macro-character: first argument must be a character")
	}
	nonTerm := false
	if len(args) >= 2 {
		nonTerm = isTruthy(args[1])
	}
	rt := currentReadtable
	if len(args) >= 3 && isReadtable(args[2]) {
		rt = args[2].readtable
	}
	// Register the dispatch character itself as a macro
	rt.macroFns[ch] = &macroEntry{
		goFn:        nil, // dispatch handled in parser
		terminating: !nonTerm,
	}
	// Initialize dispatch table if needed
	if rt.dispFns == nil {
		rt.dispFns = make(map[rune]*macroEntry)
	}
	return vbool(true), nil
}

func builtinSetDispatchMacroCharacter(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("set-dispatch-macro-character: need disp-char, sub-char, and function")
	}
	var dispCh rune
	if args[0].typ == VChar {
		dispCh = args[0].ch
	} else {
		return nil, fmt.Errorf("set-dispatch-macro-character: first argument must be a character")
	}
	var subCh rune
	if args[1].typ == VChar {
		subCh = args[1].ch
	} else {
		return nil, fmt.Errorf("set-dispatch-macro-character: second argument must be a character")
	}
	fn := args[2]
	if fn.typ != VFunc && fn.typ != VPrim {
		return nil, fmt.Errorf("set-dispatch-macro-character: third argument must be a function")
	}
	rt := currentReadtable
	if len(args) >= 4 && isReadtable(args[3]) {
		rt = args[3].readtable
	}
	_ = dispCh // dispCh validated above (ensures it's a character)
	if rt.dispFns == nil {
		rt.dispFns = make(map[rune]*macroEntry)
	}
	rt.dispFns[subCh] = &macroEntry{
		lispFn:      fn,
		terminating: false,
	}
	return vbool(true), nil
}

func builtinGetDispatchMacroCharacter(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("get-dispatch-macro-character: need disp-char and sub-char")
	}
	var dispCh rune
	if args[0].typ == VChar {
		dispCh = args[0].ch
	} else {
		return nil, fmt.Errorf("get-dispatch-macro-character: first argument must be a character")
	}
	var subCh rune
	if args[1].typ == VChar {
		subCh = args[1].ch
	} else {
		return nil, fmt.Errorf("get-dispatch-macro-character: second argument must be a character")
	}
	rt := currentReadtable
	if len(args) >= 3 && isReadtable(args[2]) {
		rt = args[2].readtable
	}
	_ = dispCh // dispCh validated above
	if rt.dispFns == nil {
		return vnil(), nil
	}
	entry, ok := rt.dispFns[subCh]
	if !ok || entry == nil {
		return vnil(), nil
	}
	if entry.lispFn != nil {
		return entry.lispFn, nil
	}
	return vnil(), nil
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

// -------- coerce improvements --------
func builtinCoerce(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("coerce: need object and result-type")
	}
	obj := args[0]
	resultType := args[1]
	typeStr := ""
	typeSub := ""
	if resultType.typ == VSym {
		typeStr = strings.ToLower(resultType.str)
	} else if isPair(resultType) && resultType.car != nil && resultType.car.typ == VSym {
		// Compound type specifier like (complex float)
		typeStr = strings.ToLower(resultType.car.str)
		if isPair(resultType.cdr) && resultType.cdr.car != nil && resultType.cdr.car.typ == VSym {
			typeSub = strings.ToLower(resultType.cdr.car.str)
		}
	}

	switch typeStr {
	case "string":
		if obj.typ == VStr {
			return obj, nil
		}
		if obj.typ == VChar {
			return vstr(string(obj.ch)), nil
		}
		if obj.typ == VSym {
			return vstr(obj.str), nil
		}
		if obj.typ == VPair || isNil(obj) {
			// list of characters/numbers/symbols/strings -> string
			var sb strings.Builder
			cur := obj
			for !isNil(cur) {
				if cur.typ == VPair {
					elt := cur.car
					if elt.typ == VChar {
						sb.WriteRune(elt.ch)
					} else if elt.typ == VNum {
						sb.WriteRune(rune(toNum(elt)))
					} else if elt.typ == VSym {
						sb.WriteString(elt.str)
					} else if elt.typ == VStr {
						sb.WriteString(elt.str)
					}
					cur = cur.cdr
				} else {
					break
				}
			}
			return vstr(sb.String()), nil
		}
		elems := seqToList(obj)
		var sb2 strings.Builder
		for _, v := range elems {
			if v.typ == VChar {
				sb2.WriteRune(v.ch)
			} else {
				sb2.WriteString(ToString(v))
			}
		}
		return vstr(sb2.String()), nil
	case "list", ":list":
		if obj.typ == VPair || isNil(obj) {
			return obj, nil
		}
		if obj.typ == VComplex {
			return listFromSlice([]*Value{vnum(obj.num), vnum(obj.imag)}), nil
		}
		if obj.typ == VStr {
			return listFromSlice(stringToCharList(obj.str)), nil
		}
		return listFromSlice(seqToList(obj)), nil
	case "float", ":float", "single-float", "double-float":
		if obj.typ != VNum && obj.typ != VRat && obj.typ != VBigInt && obj.typ != VComplex {
			return nil, fmt.Errorf("coerce: %v cannot be coerced to type %s", ToString(obj), typeStr)
		}
		return vfloat(toNum(obj)), nil
	case "rational", "ratio", ":rational", ":ratio":
		if obj.typ == VRat {
			return obj, nil
		}
		if obj.typ == VNum {
			n := toNum(obj)
			if n == float64(int(n)) {
				return vrat(int64(n), 1), nil
			}
			return toRational(n), nil
		}
		return nil, fmt.Errorf("coerce: cannot coerce to rational")
	case "complex", ":complex", "COMPLEX":
		// Check for compound type specifier: (complex float), (complex single-float), (complex double-float)
		switch typeSub {
		case "float", "single-float", "FLOAT", "SINGLE-FLOAT":
			if obj.typ == VComplex {
				r := float64(float32(obj.num))
				i := float64(float32(obj.imag))
				return vcomplexAlways(r, i), nil
			}
			r := float64(float32(toNum(obj)))
			return vcomplexAlways(r, 0), nil
		case "double-float", "DOUBLE-FLOAT":
			if obj.typ == VComplex {
				r := toNum(obj)
				i := obj.imag
				return vcomplexAlways(r, i), nil
			}
			return vcomplexAlways(toNum(obj), 0), nil
		case "rational", "integer", "RATIONAL", "INTEGER":
			// (complex rational) or (complex integer) - must always produce a VComplex
			if obj.typ == VComplex {
				return obj, nil
			}
			return vcomplexAlways(toNum(obj), 0), nil
		default:
			// Plain (complex) or unknown subtype - use vcomplex which simplifies #c(x 0) to x
			if obj.typ == VComplex {
				return obj, nil
			}
			return vcomplex(toNum(obj), 0), nil
		}
	case "character", ":character":
		if obj.typ == VChar {
			return obj, nil
		}
		if obj.typ == VStr && len(obj.str) == 1 {
			return vchar([]rune(obj.str)[0]), nil
		}
		if obj.typ == VStr && len(obj.str) > 1 {
			return nil, fmt.Errorf("coerce: string has more than one character")
		}
		if obj.typ == VNum {
			return vchar(rune(int(toNum(obj)))), nil
		}
		// Symbol designator: (coerce 'a 'character) => #\a
		if obj.typ == VSym && len(obj.str) == 1 {
			return vchar(rune(obj.str[0])), nil
		}
		return nil, fmt.Errorf("coerce: cannot coerce to character")
	case "standard-char", "base-char", ":standard-char", ":base-char":
		// Coerce to character, then verify it's a standard-char/base-char
		var ch rune
		switch obj.typ {
		case VChar:
			ch = obj.ch
		case VStr:
			if len(obj.str) == 0 {
				return nil, fmt.Errorf("coerce: string is empty")
			}
			ch = []rune(obj.str)[0]
		case VNum:
			ch = rune(int(toNum(obj)))
		default:
			return nil, fmt.Errorf("coerce: cannot coerce to %s", typeStr)
		}
		// Check if it's a standard-char
		if !isStandardChar(ch) {
			return nil, fmt.Errorf("coerce: %c is not of type %s", ch, strings.ToUpper(typeStr))
		}
		return vchar(ch), nil
	case "function", ":function":
		if obj.typ == VFunc || obj.typ == VPrim {
			return obj, nil
		}
		// (coerce 'name 'function) - look up function by name
		if obj.typ == VSym {
			fn, err := globalEnv.Get(obj.str)
			if err == nil && (fn.typ == VFunc || fn.typ == VPrim) {
				return fn, nil
			}
		}
		return nil, fmt.Errorf("coerce: cannot coerce to function")
	case "integer", ":integer":
		if obj.typ != VNum && obj.typ != VRat && obj.typ != VBigInt {
			return nil, fmt.Errorf("coerce: %v cannot be coerced to type INTEGER", ToString(obj))
		}
		n := toNum(obj)
		return vnum(math.Floor(n)), nil
	case "sequence", ":sequence":
		return obj, nil
	case "vector", ":vector", "simple-vector", ":simple-vector":
		if obj.typ == VArray && len(obj.array.dims) == 1 {
			return obj, nil
		}
		var elems []*Value
		if obj.typ == VStr {
			elems = stringToCharList(obj.str)
		} else {
			elems = seqToList(obj)
		}
		arr := &LispArray{dims: []int{len(elems)}, elements: elems, fillPtr: -1, adjustable: false}
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	case "bit-vector", "simple-bit-vector", ":bit-vector", ":simple-bit-vector":
		// Bit vector: 1D array containing only 0s and 1s
		var elems []*Value
		if obj.typ == VArray && len(obj.array.dims) == 1 {
			elems = obj.array.elements
		} else if obj.typ == VStr {
			// String of 0/1 characters
			for _, ch := range obj.str {
				if ch == '0' {
					elems = append(elems, vnum(0))
				} else if ch == '1' {
					elems = append(elems, vnum(1))
				} else {
					return nil, fmt.Errorf("coerce: string contains non-bit character %c", ch)
				}
			}
		} else {
			elems = seqToList(obj)
		}
		// Verify all elements are 0 or 1
		for i, e := range elems {
			if e.typ != VNum {
				return nil, fmt.Errorf("coerce: element %d is not a number", i)
			}
			n := toNum(e)
			if n != 0 && n != 1 {
				return nil, fmt.Errorf("coerce: element %d is not a bit (%d)", i, int(n))
			}
		}
		arr := &LispArray{dims: []int{len(elems)}, elements: elems, fillPtr: -1, adjustable: false}
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	case "array", ":array":
		if obj.typ == VArray {
			return obj, nil
		}
		var elems []*Value
		if obj.typ == VStr {
			elems = stringToCharList(obj.str)
		} else {
			elems = seqToList(obj)
		}
		arr := &LispArray{dims: []int{len(elems)}, elements: elems, fillPtr: -1, adjustable: false}
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	case "simple-array":
		// (coerce obj '(simple-array type (dim))) or (coerce obj 'simple-array)
		if obj.typ == VArray {
			return obj, nil
		}
		var elems []*Value
		if obj.typ == VStr {
			elems = stringToCharList(obj.str)
		} else {
			elems = seqToList(obj)
		}
		arr := &LispArray{dims: []int{len(elems)}, elements: elems, fillPtr: -1, adjustable: false}
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	case "real":
		if obj.typ == VComplex {
			return nil, fmt.Errorf("coerce: cannot coerce complex to type REAL")
		}
		return obj, nil
	case "number":
		return obj, nil
	case "symbol":
		if obj.typ == VSym {
			return obj, nil
		}
		if obj.typ == VStr {
			return vsym(obj.str), nil
		}
		return nil, fmt.Errorf("coerce: cannot coerce to symbol")
	default:
		return nil, fmt.Errorf("coerce: unsupported result-type %s", typeStr)
	}
}

func stringToCharList(s string) []*Value {
	runes := []rune(s)
	result := make([]*Value, len(runes))
	for i, r := range runes {
		result[i] = vchar(r)
	}
	return result
}

// character takes a character designator and returns the character.
// Designators: character, string of length 1, symbol of length 1, integer (code point).
func builtinCharacter(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("character: need a character designator")
	}
	designator := args[0]
	if designator.typ == VChar {
		return designator, nil
	}
	if designator.typ == VStr && len(designator.str) == 1 {
		return vchar([]rune(designator.str)[0]), nil
	}
	if designator.typ == VNum {
		return vchar(rune(int(toNum(designator)))), nil
	}
	if designator.typ == VSym && len(designator.str) == 1 {
		return vchar(rune(designator.str[0])), nil
	}
	return nil, fmt.Errorf("character: %v is not a character designator", designator)
}

// constantp returns true if the form is a constant at compile time.
// Constants are: numbers, characters, strings, symbols with constant values,
// and lists whose car is a special operator like quote.
func builtinConstantp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("constantp: need a form")
	}
	form := args[0]
	if isConstant(form) {
		return vbool(true), nil
	}
	return vbool(false), nil
}

func isConstant(v *Value) bool {
	if v == nil {
		return false
	}
	// Numbers, characters, strings are self-evaluating constants
	if v.typ == VNum || v.typ == VChar || v.typ == VStr {
		return true
	}
	// Quoted forms: (quote x)
	if isPair(v) && v.car != nil && v.car.typ == VSym {
		symName := v.car.str
		if symName == "QUOTE" || symName == "FUNCTION" {
			return true
		}
	}
	return false
}

// -------- ANSI CL Environment Inquiry Functions --------

// variable-information returns information about a variable binding.
// Returns (values binding-type local-p decls), where binding-type is
// :SPECIAL, :LEXICAL, :SYMBOL-MACRO, :CONSTANT, or NIL.
func builtinVariableInformation(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("variable-information: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("variable-information: need a symbol")
	}
	name := sym.str
	// Check if it's a constant (like T, NIL, PI, etc.)
	constants := map[string]bool{
		"T": true, "NIL": true, "PI": true,
		"MOST-POSITIVE-FIXNUM": true, "MOST-NEGATIVE-FIXNUM": true,
		"CHAR-CODE-LIMIT": true,
	}
	if constants[name] {
		return multiVal(vsym(":CONSTANT"), vbool(false), vnil()), nil
	}
	// Check if it's a special variable (*var* pattern)
	if strings.HasPrefix(name, "*") && strings.HasSuffix(name, "*") && len(name) > 1 {
		return multiVal(vsym(":SPECIAL"), vbool(false), vnil()), nil
	}
	// Check global environment
	_, err := globalEnv.Get(name)
	if err != nil {
		// Not bound at all
		return multiVal(vnil(), vbool(false), vnil()), nil
	}
	// Default: treat as special (global) binding
	return multiVal(vsym(":SPECIAL"), vbool(false), vnil()), nil
}

// function-information returns information about a function binding.
// Returns (values binding-type local-p decls), where binding-type is
// :FUNCTION, :MACRO, :SPECIAL-FORM, or NIL.
func builtinFunctionInformation(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("function-information: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("function-information: need a symbol")
	}
	name := sym.str
	// Check if it's a special operator
	specialOps := map[string]bool{
		"IF": true, "QUOTE": true, "SETQ": true, "BLOCK": true, "RETURN-FROM": true,
		"LET": true, "LET*": true, "PROGN": true, "TAGBODY": true, "GO": true,
		"FLET": true, "LABELS": true, "MACROLET": true, "FUNCTION": true,
		"MULTIPLE-VALUE-BIND": true, "MULTIPLE-VALUE-PROG1": true,
		"CATCH": true, "THROW": true, "UNWIND-PROTECT": true,
		"THE": true, "LOCALLY": true, "EVAL-WHEN": true,
		"SYMBOL-MACROLET": true, "LOAD-TIME-VALUE": true,
	}
	if specialOps[name] {
		return multiVal(vsym(":SPECIAL-FORM"), vbool(false), vnil()), nil
	}
	// Check global environment for function/macro
	fn, err := globalEnv.Get(name)
	if err != nil {
		return multiVal(vnil(), vbool(false), vnil()), nil
	}
	if fn.typ == VMacro {
		return multiVal(vsym(":MACRO"), vbool(false), vnil()), nil
	}
	if fn.typ == VPrim || fn.typ == VFunc || fn.typ == VGeneric {
		return multiVal(vsym(":FUNCTION"), vbool(false), vnil()), nil
	}
	return multiVal(vnil(), vbool(false), vnil()), nil
}

// declaration-information returns information about a declaration.
// Returns (values info), where info is declaration-specific.
func builtinDeclarationInformation(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("declaration-information: need a declaration specifier")
	}
	spec := args[0]
	if spec.typ != VSym {
		return multiVal(vnil(), vbool(false)), nil
	}
	name := strings.ToUpper(spec.str)
	switch name {
	case "OPTIMIZE":
		// Return default optimization qualities
		return multiVal(list(
			list(vsym("SPEED"), vnum(1)),
			list(vsym("SAFETY"), vnum(1)),
			list(vsym("DEBUG"), vnum(1)),
			list(vsym("SPACE"), vnum(1)),
			list(vsym("COMPILATION-SPEED"), vnum(1)),
		), vbool(false)), nil
	case "DECLARATION":
		// Return known declaration names
		return multiVal(list(vsym("OPTIMIZE"), vsym("DECLARATION"), vsym("DYNAMIC-EXTENT"), vsym("TYPE"), vsym("FTYPE"), vsym("NOTINLINE"), vsym("INLINE"), vsym("SPECIAL")), vbool(false)), nil
	default:
		return multiVal(vnil(), vbool(false)), nil
	}
}

func builtinIsqrt(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("isqrt: need a number")
	}
	n := toNum(args[0])
	if n < 0 {
		return nil, fmt.Errorf("isqrt: negative argument")
	}
	r := int(math.Sqrt(n))
	return vnum(float64(r)), nil
}

// -------- ANSI CL floating-point introspection --------

func builtinDecodeFloat(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("decode-float: need a float")
	}
	f := toNum(args[0])
	if f == 0 {
		return multiVal(vfloat(0), vnum(0), vnum(1)), nil
	}
	sign := 1.0
	if f < 0 {
		sign = -1.0
		f = -f
	}
	mantissa, exp := math.Frexp(f)
	return multiVal(vfloat(mantissa*sign), vnum(float64(exp)), vnum(1.0)), nil
}

func builtinIntegerDecodeFloat(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("integer-decode-float: need a float")
	}
	f := toNum(args[0])
	if f == 0 {
		return multiVal(vnum(0), vnum(0), vnum(1)), nil
	}
	sign := float64(1)
	if f < 0 {
		sign = -1
		f = -f
	}
	mantissa, exp := math.Frexp(f)
	intSig := int64(mantissa * (1 << 53))
	intExp := exp - 53
	return multiVal(vnum(float64(intSig)), vnum(float64(intExp)), vnum(sign)), nil
}

func builtinScaleFloat(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("scale-float: need float and integer")
	}
	f := toNum(args[0])
	n := toNum(args[1])
	return vfloat(f * math.Pow(2, n)), nil
}

func builtinFloatRadix(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("float-radix: need a float")
	}
	return vnum(2), nil
}

func builtinFloatDigits(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("float-digits: need a float")
	}
	return vnum(53), nil
}

func builtinFloatPrecision(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("float-precision: need a float")
	}
	f := toNum(args[0])
	if f == 0 {
		return vnum(0), nil
	}
	return vnum(53), nil
}

// -------- Advanced format --------

type fmtState struct {
	ctrl      string
	pos       int
	args      []*Value
	argIdx    int
	buf       strings.Builder
	escaped   bool
	remaining int // items remaining in current ~{ iteration (-1 = not in iteration)
}

func (fs *fmtState) done() bool { return fs.pos >= len(fs.ctrl) }
func (fs *fmtState) peek() byte {
	if fs.done() {
		return 0
	}
	return fs.ctrl[fs.pos]
}
func (fs *fmtState) next() byte {
	c := fs.ctrl[fs.pos]
	fs.pos++
	return c
}

func (fs *fmtState) popArg() *Value {
	if fs.argIdx < len(fs.args) {
		v := fs.args[fs.argIdx]
		fs.argIdx++
		return v
	}
	return vnil()
}

// formatBigIntBase formats a big.Int in the given base (2, 8, 16).
func formatBigIntBase(n *big.Int, base int) string {
	if base >= 2 && base <= 36 {
		return new(big.Int).Set(n).Text(base)
	}
	return new(big.Int).Set(n).Text(10)
}

func formatRoman(n int) string {
	if n <= 0 || n >= 4000 {
		return ""
	}
	vals := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	romans := []string{"m", "cm", "d", "cd", "c", "xc", "l", "xl", "x", "ix", "v", "iv", "i"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(romans[i])
			n -= v
		}
	}
	return b.String()
}

func formatRomanUpper(n int) string {
	return strings.ToUpper(formatRoman(n))
}

func formatOldRoman(n int) string {
	// Old-style Roman: like regular Roman but uses simpler additive notation
	if n <= 0 || n >= 4000 {
		return ""
	}
	vals := []int{1000, 500, 100, 50, 10, 5, 1}
	romans := []string{"M", "D", "C", "L", "X", "V", "I"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(romans[i])
			n -= v
		}
	}
	return b.String()
}

// formatCardinal converts a number to English cardinal word form.
// 0 -> "zero", 1 -> "one", 21 -> "twenty-one", 123 -> "one hundred twenty-three"
func formatCardinal(n int) string {
	if n == 0 {
		return "zero"
	}
	if n < 0 {
		return "minus " + formatCardinalPositive(-n)
	}
	return formatCardinalPositive(n)
}

func formatCardinalPositive(n int) string {
	if n == 0 {
		return ""
	}
	if n >= 1000000000 {
		return formatCardinalHelper(n/1000000000, "billion", formatCardinalPositive(n%1000000000))
	}
	if n >= 1000000 {
		return formatCardinalHelper(n/1000000, "million", formatCardinalPositive(n%1000000))
	}
	if n >= 1000 {
		return formatCardinalHelper(n/1000, "thousand", formatCardinalPositive(n%1000))
	}
	if n >= 100 {
		return formatCardinalHelper(n/100, "hundred", formatCardinalPositive(n%100))
	}
	tens := []string{"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"}
	teens := []string{"ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen"}
	units := []string{"", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine"}
	switch {
	case n >= 20:
		s := tens[n/10]
		if n%10 != 0 {
			s += "-" + units[n%10]
		}
		return s
	case n >= 10:
		return teens[n-10]
	default:
		return units[n]
	}
}

func formatCardinalHelper(val int, name, rest string) string {
	s := formatCardinalPositive(val) + " " + name
	if rest != "" {
		s += " " + rest
	}
	return s
}

// formatOrdinal converts a number to English ordinal word form.
// 0 -> "zeroth", 1 -> "first", 21 -> "twenty-first", 123 -> "one hundred twenty-third"
func formatOrdinal(n int) string {
	if n == 0 {
		return "zeroth"
	}
	if n < 0 {
		return "minus " + formatOrdinalPositive(-n)
	}
	return formatOrdinalPositive(n)
}

func formatOrdinalPositive(n int) string {
	ordinals := []string{
		"zeroth", "first", "second", "third", "fourth", "fifth",
		"sixth", "seventh", "eighth", "ninth", "tenth",
		"eleventh", "twelfth", "thirteenth", "fourteenth", "fifteenth",
		"sixteenth", "seventeenth", "eighteenth", "nineteenth",
		"twentieth", "twenty-first", "twenty-second", "twenty-third",
		"twenty-fourth", "twenty-fifth", "twenty-sixth", "twenty-seventh",
		"twenty-eighth", "twenty-ninth", "thirtieth", "thirty-first",
	}
	if n < len(ordinals) {
		return ordinals[n]
	}
	// For larger numbers, use cardinal form with ordinal ending on last word
	cardinal := formatCardinalPositive(n)
	words := strings.Split(cardinal, " ")
	last := words[len(words)-1]
	ordLast := lastOrdinal(last)
	if ordLast == last {
		// Fallback: just append "th"
		ordLast = last + "th"
	}
	words[len(words)-1] = ordLast
	return strings.Join(words, " ")
}

func lastOrdinal(cardinal string) string {
	ordinals := map[string]string{
		"zero": "zeroth", "one": "first", "two": "second", "three": "third",
		"four": "fourth", "five": "fifth", "six": "sixth", "seven": "seventh",
		"eight": "eighth", "nine": "ninth", "ten": "tenth",
		"eleven": "eleventh", "twelve": "twelfth", "thirteen": "thirteenth",
		"fourteen": "fourteenth", "fifteen": "fifteenth", "sixteen": "sixteenth",
		"seventeen": "seventeenth", "eighteen": "eighteenth", "nineteen": "nineteenth",
		"twenty": "twentieth", "thirty": "thirtieth", "forty": "fortieth",
		"fifty": "fiftieth", "sixty": "sixtieth", "seventy": "seventieth",
		"eighty": "eightieth", "ninety": "ninetieth",
	}
	// Check if cardinal ends with hyphenated word like "twenty-one"
	if idx := strings.LastIndex(cardinal, "-"); idx != -1 {
		lastWord := cardinal[idx+1:]
		if ord, ok := ordinals[lastWord]; ok {
			return cardinal[:idx+1] + ord
		}
	}
	if ord, ok := ordinals[cardinal]; ok {
		return ord
	}
	return cardinal
}

func (fs *fmtState) parseFmtDirective() (params []interface{}, colon, at bool, cmd byte) {
	colon = false
	at = false
	params = nil
	gotValue := false
	for !fs.done() {
		c := fs.peek()
		if c == ':' {
			colon = true
			fs.next()
		} else if c == '@' {
			at = true
			fs.next()
		} else if c == '\'' {
			fs.next()
			if !fs.done() {
				params = append(params, fs.next())
			}
			gotValue = true
		} else if c == 'V' || c == 'v' {
			fs.next()
			params = append(params, 'V')
			gotValue = true
		} else if c == '#' {
			fs.next()
			params = append(params, '#')
			gotValue = true
		} else if c >= '0' && c <= '9' {
			n := 0
			for !fs.done() && fs.peek() >= '0' && fs.peek() <= '9' {
				n = n*10 + int(fs.next()-'0')
			}
			params = append(params, n)
			gotValue = true
		} else if c == ',' {
			fs.next()
			if !gotValue {
				params = append(params, -1)
			}
			gotValue = false
		} else {
			break
		}
	}
	if !fs.done() {
		cmd = fs.next()
	}
	return
}

func (fs *fmtState) getParam(params []interface{}, idx, defaultVal int) int {
	if idx < len(params) {
		if v, ok := params[idx].(int); ok {
			return v
		}
	}
	// Check for V param
	if idx < len(params) {
		if _, ok := params[idx].(byte); ok {
			arg := fs.popArg()
			if arg.typ == VNum {
				return int(toNum(arg))
			}
			return defaultVal
		}
	}
	return defaultVal
}

func (fs *fmtState) getCharParam(params []interface{}, idx int, defaultVal byte) byte {
	if idx < len(params) {
		switch v := params[idx].(type) {
		case byte:
			return v
		case int:
			return byte(v)
		}
	}
	return defaultVal
}

func builtinFormat(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("format: need stream and control-string")
	}
	stream := args[0]
	ctrl := args[1]
	if ctrl.typ != VStr {
		return nil, fmt.Errorf("format: control-string must be a string")
	}
	fs := &fmtState{
		ctrl: ctrl.str,
		args: args[2:],
	}
	formatRun(fs)
	result := fs.buf.String()
	if stream == globalEnv.bindings["#t"] {
		fmt.Print(result)
		return vnil(), nil
	}
	// If stream is a real stream (not nil), write to it
	if stream.typ == VStream && stream.stream != nil && stream.stream.isOutput {
		if stream.stream.isString && stream.stream.strBuf != nil {
			stream.stream.strBuf.WriteString(result)
		}
		return vnil(), nil
	}
	return vstr(result), nil
}

func newFmtState(ctrl string, args []*Value, argIdx int) *fmtState {
	return &fmtState{ctrl: ctrl, args: args, argIdx: argIdx, remaining: -1}
}

func formatRun(fs *fmtState) {
	for !fs.done() && !fs.escaped {
		c := fs.peek()
		if c == '~' {
			fs.next()
			if fs.done() {
				fs.buf.WriteByte('~')
				return
			}
			formatDispatch(fs)
		} else {
			fs.buf.WriteByte(fs.next())
		}
	}
}

func formatDispatch(fs *fmtState) {
	params, colon, at, cmd := fs.parseFmtDirective()
	// Format directives are case-insensitive
	if cmd >= 'a' && cmd <= 'z' {
		cmd = cmd - 'a' + 'A'
	}
	switch cmd {
	case 'A':
		arg := fs.popArg()
		mincol := fs.getParam(params, 0, 0)
		colinc := fs.getParam(params, 1, 1)
		minpad := fs.getParam(params, 2, 0)
		padchar := fs.getCharParam(params, 3, ' ')
		s := princToString(arg)
		if isNil(arg) && colon {
			s = "()"
		}
		padlen := 0
		if int(mincol) > len(s) {
			padlen = int(mincol) - len(s)
			if int(minpad) > padlen {
				padlen = int(minpad)
			}
			if int(colinc) > 1 {
				for padlen < int(mincol)-len(s) || (padlen-int(minpad))%int(colinc) != 0 {
					padlen++
				}
			}
		}
		if at {
			for i := 0; i < padlen; i++ {
				fs.buf.WriteByte(byte(padchar))
			}
			fs.buf.WriteString(s)
		} else {
			fs.buf.WriteString(s)
			for i := 0; i < padlen; i++ {
				fs.buf.WriteByte(byte(padchar))
			}
		}
	case 'S':
		arg := fs.popArg()
		mincol := fs.getParam(params, 0, 0)
		padchar := fs.getCharParam(params, 3, ' ')
		s := writeToString(arg)
		padlen := 0
		if int(mincol) > len(s) {
			padlen = int(mincol) - len(s)
		}
		if at {
			for i := 0; i < padlen; i++ {
				fs.buf.WriteByte(byte(padchar))
			}
			fs.buf.WriteString(s)
		} else {
			fs.buf.WriteString(s)
			for i := 0; i < padlen; i++ {
				fs.buf.WriteByte(byte(padchar))
			}
		}
	case 'D':
		width := fs.getParam(params, 0, 0)
		padchar := fs.getCharParam(params, 1, ' ')
		arg := fs.popArg()
		var s string
		if arg.typ == VBigInt {
			s = arg.bigInt.String()
		} else {
			s = strconv.FormatInt(int64(toNum(arg)), 10)
		}
		n := 0
		if arg.typ == VBigInt {
			n = arg.bigInt.Sign()
		} else {
			n = int(toNum(arg))
		}
		if n >= 0 && at {
			s = "+" + s
		}
		for len(s) < width {
			s = string(padchar) + s
		}
		if colon && !at {
			var b strings.Builder
			for i, c := range s {
				if i > 0 && (len(s)-i)%3 == 0 && c != '-' {
					b.WriteByte(',')
				}
				b.WriteRune(c)
			}
			s = b.String()
		}
		fs.buf.WriteString(s)
	case 'B':
		arg := fs.popArg()
		var s string
		if arg.typ == VBigInt {
			s = formatBigIntBase(arg.bigInt, 2)
		} else {
			s = strconv.FormatInt(int64(toNum(arg)), 2)
		}
		if at {
			fs.buf.WriteString("#b" + s)
		} else {
			fs.buf.WriteString(s)
		}
	case 'O':
		arg := fs.popArg()
		var s string
		if arg.typ == VBigInt {
			s = formatBigIntBase(arg.bigInt, 8)
		} else {
			s = strconv.FormatInt(int64(toNum(arg)), 8)
		}
		if at {
			fs.buf.WriteString("#o" + s)
		} else {
			fs.buf.WriteString(s)
		}
	case 'X':
		arg := fs.popArg()
		var s string
		if arg.typ == VBigInt {
			s = formatBigIntBase(arg.bigInt, 16)
		} else {
			s = strconv.FormatInt(int64(toNum(arg)), 16)
		}
		if at {
			fs.buf.WriteString("#x" + s)
		} else {
			fs.buf.WriteString(s)
		}
	case 'F':
		decimals := fs.getParam(params, 1, -1)
		arg := fs.popArg()
		f := toNum(arg)
		var s string
		if decimals >= 0 {
			s = strconv.FormatFloat(f, 'f', decimals, 64)
		} else {
			s = strconv.FormatFloat(f, 'f', -1, 64)
		}
		// ~F must always produce a float: ensure decimal point is present
		if !strings.Contains(s, ".") {
			s = s + ".0"
		}
		fs.buf.WriteString(s)
	case '$':
		// ~$, ~:$, ~@$ - dollar float formatting
		// Params: d n w padchar
		// d = digits after decimal (default 2)
		// n = minimum digits before decimal point (default 1)
		// w = minimum field width (default 0)
		// padchar = padding character (default space)
		d := fs.getParam(params, 0, 2)
		n := fs.getParam(params, 1, 1)
		w := fs.getParam(params, 2, 0)
		padchar := fs.getCharParam(params, 3, ' ')
		arg := fs.popArg()
		f := toNum(arg)
		s := strconv.FormatFloat(f, 'f', int(d), 64)
		// Ensure minimum digits before decimal point
		if idx := strings.Index(s, "."); idx >= 0 {
			beforeDot := s[:idx]
			sign := ""
			if len(beforeDot) > 0 && (beforeDot[0] == '-' || beforeDot[0] == '+') {
				sign = string(beforeDot[0])
				beforeDot = beforeDot[1:]
			}
			for len(beforeDot) < int(n) {
				beforeDot = "0" + beforeDot
			}
			s = sign + beforeDot + s[idx:]
		}
		// Handle @ modifier: always print sign
		if at && f >= 0 {
			s = "+" + s
		}
		// Handle colon modifier: sign appears before padding
		if colon {
			// ~:$ sign before padding
			padlen := int(w) - len(s)
			if padlen < 0 {
				padlen = 0
			}
			signPart := ""
			if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
				signPart = string(s[0])
				s = s[1:]
			}
			fs.buf.WriteString(signPart)
			for i := 0; i < padlen; i++ {
				fs.buf.WriteRune(rune(padchar))
			}
			fs.buf.WriteString(s)
		} else {
			padlen := int(w) - len(s)
			if padlen < 0 {
				padlen = 0
			}
			for i := 0; i < padlen; i++ {
				fs.buf.WriteRune(rune(padchar))
			}
			fs.buf.WriteString(s)
		}
	case 'E':
		// ~E: scientific notation
		decimals := fs.getParam(params, 1, -1)
		eMinDigits := fs.getParam(params, 2, 1)
		arg := fs.popArg()
		f := toNum(arg)
		prec := -1
		if decimals >= 0 {
			prec = int(decimals) + 1
		}
		s := strconv.FormatFloat(f, 'E', prec, 64)
		idx := strings.Index(s, "E")
		if idx < 0 {
			fs.buf.WriteString(s)
			break
		}
		mantissa := s[:idx]
		expStr := s[idx+1:]
		// Ensure mantissa has a decimal point
		if !strings.Contains(mantissa, ".") {
			mantissa += ".0"
		}
		// Normalize exponent: strip leading zeros
		expSign := ""
		if len(expStr) > 0 && (expStr[0] == '+' || expStr[0] == '-') {
			expSign = string(expStr[0])
			expStr = expStr[1:]
		}
		expDigits := strings.TrimLeft(expStr, "0")
		if expDigits == "" {
			expDigits = "0"
		}
		// Pad to minimum digit count
		for len(expDigits) < int(eMinDigits) {
			expDigits = "0" + expDigits
		}
		fs.buf.WriteString(mantissa + "E" + expSign + expDigits)
	case 'R':
		arg := fs.popArg()
		n := int(toNum(arg))
		// ~nR with radix parameter: print in base n (ANSI CL)
		if len(params) >= 1 && !colon && !at {
			radix := int(fs.getParam(params, 0, 10))
			if radix < 2 || radix > 36 {
				fs.buf.WriteString(formatCardinal(n))
			} else {
				fs.buf.WriteString(formatBigIntBase(big.NewInt(int64(n)), radix))
			}
			break
		}
		switch {
		case at && colon:
			// ~:@R: old-style Roman numerals
			fs.buf.WriteString(formatOldRoman(n))
		case at:
			// ~@R: Roman numerals (uppercase)
			fs.buf.WriteString(formatRomanUpper(n))
		case colon:
			// ~:R: ordinal
			fs.buf.WriteString(formatOrdinal(n))
		default:
			// ~R: cardinal
			fs.buf.WriteString(formatCardinal(n))
		}
	case 'C':
		arg := fs.popArg()
		if arg.typ == VChar {
			if at {
				// ~@C: print with escape syntax like prin1
				fs.buf.WriteString(ToString(arg))
			} else if colon {
				// ~:C: spell out the character name for non-printing chars
				ch := arg.ch
				switch ch {
				case ' ':
					fs.buf.WriteString("Space")
				case '\n':
					fs.buf.WriteString("Newline")
				case '\t':
					fs.buf.WriteString("Tab")
				case '\r':
					fs.buf.WriteString("Return")
				case '\x08':
					fs.buf.WriteString("Backspace")
				case '\x7f':
					fs.buf.WriteString("Rubout")
				case '\f':
					fs.buf.WriteString("Page")
				default:
					if ch < ' ' || ch == '\x7f' {
						// Non-printing: use #\name form
						fs.buf.WriteString(ToString(arg))
					} else {
						fs.buf.WriteRune(ch)
					}
				}
			} else {
				// ~C: print just the character itself (like princ)
				fs.buf.WriteRune(arg.ch)
			}
		} else {
			fs.buf.WriteString(princToString(arg))
		}
	case '%':
		n := int(fs.getParam(params, 0, 1))
		for i := 0; i < n; i++ {
			fs.buf.WriteString("\n")
		}
	case '&':
		// fresh-line: output newline unless already at start of line
		if fs.buf.Len() > 0 {
			n := int(fs.getParam(params, 0, 1))
			for i := 0; i < n; i++ {
				fs.buf.WriteString("\n")
			}
		}
	case '|':
		fs.buf.WriteString("\f")
	case '~':
		fs.buf.WriteString("~")
	case 'T':
		colnum := fs.getParam(params, 0, 1)
		colinc := fs.getParam(params, 1, 1)
		current := fs.buf.Len()
		if at {
			// ~@T: output colnum spaces, then enough to reach next multiple of colinc
			for i := 0; i < int(colnum); i++ {
				fs.buf.WriteByte(' ')
			}
			current = fs.buf.Len()
			if int(colinc) > 0 {
				rem := current % int(colinc)
				if rem != 0 {
					for i := 0; i < int(colinc)-rem; i++ {
						fs.buf.WriteByte(' ')
					}
				}
			}
		} else {
			// ~T: advance to column colnum, or if already past, advance by colinc
			if current < int(colnum) {
				for i := current; i < int(colnum); i++ {
					fs.buf.WriteByte(' ')
				}
			} else if int(colinc) > 0 {
				target := current
				if target%int(colinc) != 0 {
					target = ((target / int(colinc)) + 1) * int(colinc)
				} else {
					target = current + int(colinc)
				}
				for i := current; i < target; i++ {
					fs.buf.WriteByte(' ')
				}
			}
		}
	case '*':
		n := fs.getParam(params, 0, 1)
		if at {
			// ~@* with no param: go to arg 0 (first argument)
			if len(params) == 0 {
				n = 0
			}
			fs.argIdx = n
		} else {
			fs.argIdx += n
		}
	case '?':
		if at {
			// ~@?: pop control string from args, use remaining args for recursive format
			newCtrl := fs.popArg()
			if newCtrl.typ == VStr {
				subFs := &fmtState{ctrl: newCtrl.str, args: fs.args, argIdx: fs.argIdx}
				formatRun(subFs)
				fs.buf.WriteString(subFs.buf.String())
				fs.argIdx = subFs.argIdx
			}
		} else {
			// ~?: pop control string and argument list
			newCtrl := fs.popArg()
			newArgs := fs.popArg()
			if newCtrl.typ == VStr {
				subFs := &fmtState{ctrl: newCtrl.str, args: seqToList(newArgs)}
				formatRun(subFs)
				fs.buf.WriteString(subFs.buf.String())
			}
		}
	case '(':
		// ~( ... ~) - case conversion
		depth := 1
		bodyStart := fs.pos
		for !fs.done() && depth > 0 {
			c := fs.next()
			if c == '~' && !fs.done() {
				nc := fs.next()
				if nc == '(' {
					depth++
				} else if nc == ')' {
					depth--
				}
			}
		}
		bodyEnd := fs.pos - 2
		if bodyEnd < bodyStart {
			bodyEnd = bodyStart
		}
		body := fs.ctrl[bodyStart:bodyEnd]
		subFs := newFmtState(body, fs.args, fs.argIdx)
		formatRun(subFs)
		fs.argIdx = subFs.argIdx
		s := subFs.buf.String()
		if colon && at {
			s = titleCaseString(strings.ToLower(s))
		} else if colon {
			s = strings.ToUpper(s)
		} else if at {
			if len(s) > 0 {
				s = strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
			}
		} else {
			s = strings.ToLower(s)
		}
		fs.buf.WriteString(s)
	case ')':
	case '[':
		sections := formatCollectSections(fs, ']')
		selector := fs.popArg()
		sel := int(toNum(selector))
		if sel < 0 {
			sel = 0
		}
		if sel >= len(sections) {
			sel = len(sections) - 1
		}
		if sel >= 0 && sel < len(sections) {
			subFs := &fmtState{ctrl: sections[sel], args: fs.args, argIdx: fs.argIdx}
			formatRun(subFs)
			fs.buf.WriteString(subFs.buf.String())
			fs.argIdx = subFs.argIdx
		}
	case ';', ']':
	case '{':
		body := formatCollectBody(fs, '}')
		limit := fs.getParam(params, 0, -1) // -1 means no limit
		if colon {
			listArg := fs.popArg()
			elements := seqToList(listArg)
			for i, el := range elements {
				if limit >= 0 && i >= limit {
					break
				}
				rem := len(elements) - i - 1
				subFs := &fmtState{ctrl: body, args: seqToList(el), remaining: rem}
				formatRun(subFs)
				fs.buf.WriteString(subFs.buf.String())
			}
		} else if at {
			count := 0
			prevArgIdx := fs.argIdx
			iterCount := 0
			for fs.argIdx < len(fs.args) {
				if limit >= 0 && count >= limit {
					break
				}
				if iterCount > len(fs.args)+10 {
					break
				}
				rem := len(fs.args) - fs.argIdx - 1
				subFs := &fmtState{ctrl: body, args: fs.args, argIdx: fs.argIdx, remaining: rem}
				formatRun(subFs)
				fs.buf.WriteString(subFs.buf.String())
				if subFs.argIdx <= fs.argIdx {
					break
				}
				fs.argIdx = subFs.argIdx
				if fs.argIdx == prevArgIdx {
					break
				}
				count++
				iterCount++
			}
		} else {
			listArg := fs.popArg()
			elements := seqToList(listArg)
			for i, el := range elements {
				if limit >= 0 && i >= limit {
					break
				}
				rem := len(elements) - i - 1
				subFs := &fmtState{ctrl: body, args: []*Value{el}, remaining: rem}
				formatRun(subFs)
				fs.buf.WriteString(subFs.buf.String())
			}
		}
	case '}':
	case '^':
		if fs.remaining >= 0 {
			// Inside ~{ iteration: check if more items remain
			if fs.remaining == 0 {
				fs.escaped = true
			}
		} else if fs.argIdx >= len(fs.args) {
			fs.escaped = true
		}
	case '<':
		// ~mincol,colinc,minpad,padchar<text~> — Justification
		// ~<seg1~;seg2~;seg3~> — segmented: distribute segments across width
		// Extract body between ~< and ~>
		depth := 1
		bodyStart := fs.pos
		for !fs.done() && depth > 0 {
			c := fs.next()
			if c == '~' && !fs.done() {
				nc := fs.next()
				if nc == '<' {
					depth++
				} else if nc == '>' {
					depth--
				}
			}
		}
		bodyEnd := fs.pos - 2
		if bodyEnd < bodyStart {
			bodyEnd = bodyStart
		}
		body := fs.ctrl[bodyStart:bodyEnd]

		// Check for segments separated by ~; and detect fill separator (~:;)
		segments, fillSegIdx := formatCollectJustifySegments(body)

		if !colon && !at && len(segments) <= 1 {
			// ~<~> without modifiers, no segments: process body and output
			subFs := newFmtState(body, fs.args, fs.argIdx)
			formatRun(subFs)
			fs.argIdx = subFs.argIdx
			fs.buf.WriteString(subFs.buf.String())
		} else if len(segments) > 1 {
			// Segmented justification: process each segment and distribute
			// padding between them to fill mincol
			mincol := fs.getParam(params, 0, 0)
			colinc := fs.getParam(params, 1, 1)
			padchar := fs.getCharParam(params, 3, ' ')

			// Process each segment
			processedSegs := make([]string, 0, len(segments))
			for _, seg := range segments {
				subFs := newFmtState(seg, fs.args, fs.argIdx)
				formatRun(subFs)
				fs.argIdx = subFs.argIdx
				processedSegs = append(processedSegs, subFs.buf.String())
			}

			totalContentLen := 0
			for _, s := range processedSegs {
				totalContentLen += len(s)
			}

			// Calculate total padding needed
			numGaps := len(processedSegs) - 1
			if numGaps <= 0 {
				numGaps = 1
			}
			totalPadLen := 0
			if int(mincol) > totalContentLen {
				totalPadLen = int(mincol) - totalContentLen
				if int(colinc) > 1 && numGaps > 0 {
					// Round up totalPadLen to a multiple of colinc
					for totalPadLen%int(colinc) != 0 && totalPadLen+totalContentLen < int(mincol)+int(colinc) {
						totalPadLen++
					}
				}
			}

			// Distribute padding between gaps
			gapPad := make([]int, numGaps)
			remaining := totalPadLen
			if fillSegIdx >= 0 && fillSegIdx < numGaps {
				// Fill section: all extra padding goes to the gap after fill separator
				for i := 0; i < numGaps; i++ {
					if i == fillSegIdx {
						gapPad[i] = remaining
					} else {
						gapPad[i] = 0
					}
				}
			} else {
				// Distribute evenly
				base := 0
				if numGaps > 0 {
					base = remaining / numGaps
				}
				extra := remaining - base*numGaps
				for i := 0; i < numGaps; i++ {
					gapPad[i] = base
					if i < extra {
						gapPad[i]++
					}
				}
			}

			// Output segments with padding
			for i, s := range processedSegs {
				fs.buf.WriteString(s)
				if i < numGaps {
					for j := 0; j < gapPad[i]; j++ {
						fs.buf.WriteByte(byte(padchar))
					}
				}
			}
		} else {
			// ~:@<~> or ~@<~> or ~:<~>: single-segment justification
			mincol := fs.getParam(params, 0, 0)
			colinc := fs.getParam(params, 1, 1)
			minpad := fs.getParam(params, 2, 0)
			padchar := fs.getCharParam(params, 3, ' ')

			subFs := newFmtState(body, fs.args, fs.argIdx)
			formatRun(subFs)
			fs.argIdx = subFs.argIdx
			s := subFs.buf.String()

			// Calculate padding to reach mincol
			padlen := 0
			if int(mincol) > len(s) {
				padlen = int(mincol) - len(s)
				if int(minpad) > padlen {
					padlen = int(minpad)
				}
				if int(colinc) > 1 {
					for padlen < int(mincol)-len(s) || (padlen-int(minpad))%int(colinc) != 0 {
						padlen++
					}
				}
			}

			if colon && at {
				// ~:@< : center (pad on both sides equally)
				leftPad := padlen / 2
				rightPad := padlen - leftPad
				for i := 0; i < leftPad; i++ {
					fs.buf.WriteByte(byte(padchar))
				}
				fs.buf.WriteString(s)
				for i := 0; i < rightPad; i++ {
					fs.buf.WriteByte(byte(padchar))
				}
			} else if at {
				// ~@< : right-justify (pad on left)
				for i := 0; i < padlen; i++ {
					fs.buf.WriteByte(byte(padchar))
				}
				fs.buf.WriteString(s)
			} else {
				// ~:< : left-justify (pad on right)
				fs.buf.WriteString(s)
				for i := 0; i < padlen; i++ {
					fs.buf.WriteByte(byte(padchar))
				}
			}
		}
	case '>':
	case 'P':
		n := int(toNum(fs.popArg()))
		if at {
			if n != 1 {
				fs.buf.WriteString("ies")
			} else {
				fs.buf.WriteString("y")
			}
		} else if colon {
			if n != 1 {
				fs.buf.WriteString("es")
			}
		} else {
			if n != 1 {
				fs.buf.WriteString("s")
			}
		}
	case 'G':
		// General format: scientific for very large/small, fixed otherwise
		digits := fs.getParam(params, 0, 6)
		if digits < 1 {
			digits = 1
		}
		arg := fs.popArg()
		f := toNum(arg)
		if f == 0 {
			fs.buf.WriteString("0.0")
		} else {
			absF := math.Abs(f)
			// Calculate exponent for determining format
			exp := int(math.Floor(math.Log10(absF)))
			// CL ~g: use fixed-point when -4 <= exp < digits
			if absF >= 1e16 || (absF < 1e-3 && absF > 0) || exp >= digits || exp < -4 {
				// Use exponential format with ~E-like logic
				// Format as mantissa + E + exponent, stripping trailing zeros
				prec := digits - 1
				if prec < 0 {
					prec = 0
				}
				s := strconv.FormatFloat(f, 'E', prec, 64)
				idx := strings.Index(s, "E")
				if idx < 0 {
					fs.buf.WriteString(s)
				} else {
					mantissa := s[:idx]
					expStr := s[idx+1:]
					// Strip trailing zeros from mantissa
					mantissa = strings.TrimRight(mantissa, "0")
					// If mantissa ends with '.', remove it
					mantissa = strings.TrimSuffix(mantissa, ".")
					// If mantissa is empty, use "0"
					if mantissa == "" {
						mantissa = "0"
					}
					// Ensure mantissa has a decimal point
					if !strings.Contains(mantissa, ".") {
						mantissa += ".0"
					}
					// Normalize exponent: strip leading zeros
					expSign := ""
					if len(expStr) > 0 && (expStr[0] == '+' || expStr[0] == '-') {
						expSign = string(expStr[0])
						expStr = expStr[1:]
					}
					expDigits := strings.TrimLeft(expStr, "0")
					if expDigits == "" {
						expDigits = "0"
					}
					fs.buf.WriteString(mantissa + "E" + expSign + expDigits)
				}
			} else {
				// Use fixed-point format with appropriate decimal places
				decPlaces := digits - 1 - exp
				if decPlaces < 0 {
					decPlaces = 0
				}
				s := strconv.FormatFloat(f, 'f', decPlaces, 64)
				// Strip unnecessary trailing zeros, but keep at least one decimal digit
				if dotIdx := strings.Index(s, "."); dotIdx >= 0 {
					// Remove trailing zeros after decimal point
					s = strings.TrimRight(s, "0")
					// If only '.' remains, add one '0' to keep "42."
					s = strings.TrimSuffix(s, ".")
					if s == "" || !strings.Contains(s, ".") {
						s += ".0"
					}
				} else {
					// No decimal point - add ".0" for floating-point clarity
					s += ".0"
				}
				fs.buf.WriteString(s)
			}
		}
	case 'W':
		// ~W: write format - use writeToString for canonical output with escape
		// ~:W uses princ output (no escape), ~@W uses print (with newline), ~:@W uses princ + newline
		arg := fs.popArg()
		if colon && at {
			fs.buf.WriteString(princToString(arg))
			fs.buf.WriteByte('\n')
		} else if colon {
			fs.buf.WriteString(princToString(arg))
		} else if at {
			fs.buf.WriteString(writeToString(arg))
			fs.buf.WriteByte('\n')
		} else {
			fs.buf.WriteString(writeToString(arg))
		}
	case '_':
		// Conditional newline: output a newline (simplified)
		fs.buf.WriteByte('\n')
	case 'I':
		// Indent: output spaces for pretty-printing
		n := fs.getParam(params, 0, 0)
		for i := 0; i < n; i++ {
			fs.buf.WriteByte(' ')
		}
	case '/':
		// ~/function-name/ — call a user-defined function to format
		var fnName strings.Builder
		for !fs.done() {
			c := fs.next()
			if c == '/' {
				break
			}
			fnName.WriteByte(c)
		}
		name := fnName.String()
		fnVal, fnErr := globalEnv.Get(strings.ToUpper(name))
		if fnErr != nil || fnVal == nil || (fnVal.typ != VPrim && fnVal.typ != VFunc && fnVal.typ != VGeneric) {
			fs.buf.WriteString("?")
		} else {
			arg := fs.popArg()
			colVal := vbool(colon)
			atVal := vbool(at)
			callArgs := []*Value{arg, colVal, atVal}
			for _, p := range params {
				if f64, ok := p.(float64); ok {
					callArgs = append(callArgs, vnum(f64))
				} else {
					callArgs = append(callArgs, vnum(0))
				}
			}
			result, _ := callFnOnSeq(fnVal, callArgs, nil)
			if result != nil {
				fs.buf.WriteString(princToString(result))
			}
		}
	}
}

// formatCollectJustifySegments splits a ~<...~> body by ~; separators,
// respecting nested directives. Returns segments and the index of the
// gap that follows the ~:; fill separator (-1 if none).
func formatCollectJustifySegments(body string) ([]string, int) {
	var segments []string
	fillGapIdx := -1
	depth := 0
	start := 0
	pos := 0
	for pos < len(body) {
		if body[pos] == '~' && pos+1 < len(body) {
			nc := body[pos+1]
			if nc == '<' {
				depth++
				pos += 2
			} else if nc == '>' {
				depth--
				pos += 2
			} else if nc == ':' && pos+2 < len(body) && body[pos+2] == ';' && depth == 0 {
				// ~:; fill separator
				segments = append(segments, body[start:pos])
				fillGapIdx = len(segments) // gap after this segment gets fill padding
				pos += 3
				start = pos
			} else if nc == ';' && depth == 0 {
				// ~; regular separator
				segments = append(segments, body[start:pos])
				pos += 2
				start = pos
			} else {
				pos += 2
			}
		} else {
			pos++
		}
	}
	if start < len(body) {
		segments = append(segments, body[start:])
	}
	return segments, fillGapIdx
}

func formatCollectSections(fs *fmtState, endChar byte) []string {
	var sections []string
	depth := 0
	start := fs.pos
	for !fs.done() {
		if fs.peek() == '~' {
			fs.next()
			if fs.done() {
				break
			}
			nc := fs.peek()
			if nc == '[' {
				depth++
				fs.next()
			} else if nc == ']' {
				if depth == 0 {
					sections = append(sections, fs.ctrl[start:fs.pos-1])
					fs.next()
					return sections
				}
				depth--
				fs.next()
			} else if nc == ':' {
				// Check for ~:; (default clause marker)
				pos := fs.pos
				if pos+1 < len(fs.ctrl) && fs.ctrl[pos+1] == ';' && depth == 0 {
					// ~:; — mark everything before this as section,
					// everything after as default, skip the next ~;
					sections = append(sections, fs.ctrl[start:fs.pos-1])
					fs.next() // consume ':'
					fs.next() // consume ';'
					start = fs.pos
					// Now continue collecting, but when we see the next ~; at depth 0,
					// skip it (it's the paired ~; with ~:)
					for !fs.done() {
						if fs.peek() == '~' {
							fs.next()
							if fs.done() {
								break
							}
							nc2 := fs.peek()
							if nc2 == '[' {
								depth++
								fs.next()
							} else if nc2 == ']' {
								if depth == 0 {
									sections = append(sections, fs.ctrl[start:fs.pos-1])
									fs.next()
									return sections
								}
								depth--
								fs.next()
							} else if nc2 == '{' {
								depth++
								fs.next()
							} else if nc2 == '}' {
								depth--
								fs.next()
							} else if nc2 == ';' && depth == 0 {
								// Skip the paired ~; — this is the second half of ~:;
								fs.next()
								start = fs.pos
							} else {
								fs.next()
							}
						} else {
							fs.next()
						}
					}
					sections = append(sections, fs.ctrl[start:fs.pos])
					return sections
				}
				// Not ~:; just consume
				fs.next()
			} else if nc == ';' && depth == 0 {
				sections = append(sections, fs.ctrl[start:fs.pos-1])
				fs.next()
				start = fs.pos
			} else if nc == '{' {
				depth++
				fs.next()
			} else if nc == '}' {
				depth--
				fs.next()
			} else {
				fs.next()
			}
		} else {
			fs.next()
		}
	}
	sections = append(sections, fs.ctrl[start:fs.pos])
	return sections
}

func formatCollectBody(fs *fmtState, endChar byte) string {
	depth := 0
	start := fs.pos
	for !fs.done() {
		if fs.peek() == '~' {
			fs.next()
			if fs.done() {
				break
			}
			nc := fs.next()
			if nc == '{' {
				depth++
			} else if nc == '}' {
				if depth == 0 {
					return fs.ctrl[start : fs.pos-2]
				}
				depth--
			} else if nc == '[' {
				depth++
			} else if nc == ']' {
				depth--
			}
		} else {
			fs.next()
		}
	}
	return fs.ctrl[start:fs.pos]
}

func princToString(v *Value) string {
	switch v.typ {
	case VBigInt:
		return v.bigInt.String()
	case VStr:
		return v.str
	case VChar:
		return string(v.ch)
	case VNil:
		return "nil"
	case VBool:
		if v == globalEnv.bindings["#t"] {
			return "#t"
		}
		return "#f"
	case VPair, VArray:
		// Use writeToString for circular reference detection
		return writeToString(v)
	case VInstance:
		// Check for defstruct :print-function / :print-object first
		if v.instClass != nil {
			if printFn, ok := structPrintFns[strings.ToUpper(v.instClass.str)]; ok && printFn != nil {
				// Create a string output stream and call the print function
				outStream := newStringOutput()
				_, callErr := callFnOnSeq(printFn, []*Value{v, outStream, vnum(0)}, globalEnv)
				if callErr == nil {
					return outStream.stream.strBuf.String()
				}
				// If print function fails, fall through to default
			}
		}
		// Check if this is a condition instance with format-control/format-arguments
		if v.instClass != nil && classHasAncestor(v.instClass, "condition") {
			fc := instanceSlotWithInheritance(v, "format-control")
			fa := instanceSlotWithInheritance(v, "format-arguments")
			if fc != nil && fc.typ == VStr && len(fc.str) > 0 {
				var args []*Value
				if fa != nil && fa.typ != VNil {
					cur := fa
					for cur.typ == VPair {
						args = append(args, cur.car)
						cur = cur.cdr
					}
				}
				return formatMessage(fc.str, args)
			}
		}
		return writeToString(v)
	default:
		return ToString(v)
	}
}

// -------- Printer --------
func ToString(v *Value) string {
	if v == nil {
		return "()"
	}
	switch v.typ {
	case VNil:
		return "()"
	case VNum:
		f := v.num
		if v.isFloat {
			// explicitly a float: always print with decimal point
			s := strconv.FormatFloat(f, 'g', -1, 64)
			if !strings.ContainsAny(s, ".eE") {
				s += ".0"
			}
			return s
		}
		if f == math.Trunc(f) && !math.IsInf(f, 0) && math.Abs(f) < 1e15 {
			return strconv.FormatInt(int64(f), 10)
		}
		return strconv.FormatFloat(f, 'g', -1, 64)
	case VStr:
		return `"` + strings.ReplaceAll(v.str, `"`, `\"`) + `"`
	case VSym:
		return v.str
	case VBool:
		if v == globalEnv.bindings["#t"] {
			return "#t"
		}
		return "#f"
	case VPair:
		return listToString(v)
	case VPrim:
		return "#<primitive>"
	case VFunc:
		return "#<procedure>"
	case VMacro:
		return "#<macro>"
	case VRat:
		return strconv.FormatInt(v.irat, 10) + "/" + strconv.FormatInt(v.iden, 10)
	case VComplex:
		r := formatComplexPart(v.num, v.isFloat)
		i := formatComplexPart(v.imag, v.isFloat)
		return "#c(" + r + " " + i + ")"
	case VBigInt:
		return v.bigInt.String()
	case VPathname:
		return "#P\"" + pathnameToString(v.pathname) + "\""
	case VPackage:
		if v.pkg != nil {
			return "#<PACKAGE " + v.pkg.name + ">"
		}
		return "#<PACKAGE>"
	case VReadtable:
		return "#<READTABLE>"
	case VClass:
		return "#<class " + v.str + ">"
	case VGeneric:
		return "#<generic " + v.str + ">"
	case VInstance:
		if v.instClass != nil {
			if printFn, ok := structPrintFns[strings.ToUpper(v.instClass.str)]; ok && printFn != nil {
				outStream := newStringOutput()
				_, callErr := callFnOnSeq(printFn, []*Value{v, outStream, vnum(0)}, globalEnv)
				if callErr == nil {
					return outStream.stream.strBuf.String()
				}
			}
			return "#<instance " + v.instClass.str + ">"
		}
		return "#<instance>"
	case VVHash:
		return "#<hash-table " + strconv.Itoa(v.hashTab.count) + ">"
	case VThread:
		return "#<thread " + strconv.FormatInt(int64(v.num), 10) + ">"
	case VLock:
		return "#<lock " + strconv.FormatInt(int64(v.num), 10) + ">"
	case VChar:
		switch v.ch {
		case ' ':
			return "#\\space"
		case '\n':
			return "#\\newline"
		case '\t':
			return "#\\tab"
		case '\r':
			return "#\\return"
		case '\x08':
			return "#\\backspace"
		case '\x7f':
			return "#\\rubout"
		case '\f':
			return "#\\page"
		case '\x00':
			return "#\\null"
		case '\x07':
			return "#\\bell"
		case '\x1b':
			return "#\\escape"
		case '\x01':
			return "#\\soh"
		case '\x02':
			return "#\\stx"
		case '\x03':
			return "#\\etx"
		case '\x04':
			return "#\\eot"
		case '\x05':
			return "#\\enq"
		case '\x06':
			return "#\\ack"
		case '\x15':
			return "#\\nak"
		case '\x16':
			return "#\\syn"
		case '\x17':
			return "#\\etb"
		case '\x18':
			return "#\\can"
		case '\x19':
			return "#\\em"
		case '\x1a':
			return "#\\sub"
		case '\x1c':
			return "#\\fs"
		case '\x1d':
			return "#\\gs"
		case '\x1e':
			return "#\\rs"
		case '\x1f':
			return "#\\us"
		case '\x11':
			return "#\\xon"
		case '\x13':
			return "#\\xoff"
		default:
			return "#\\" + string(v.ch)
		}
	case VStream:
		return "#<stream>"
	case VArray:
		return arrayToString(v)
	case VMultiVal:
		return listToString(cons(v.car, v.cdr))
	case VRandomState:
		return "#<random-state>"
	case VMethod:
		return "#<method " + v.str + ">"
	case VRestart:
		return "#<restart " + v.str + ">"
	default:
		return "#<unknown>"
	}
}

// -------- Circular structure printing (*print-circle*) --------

// circleState tracks visited values for circular reference detection
type circleState struct {
	seen    map[*Value]int // value -> label number
	counter int
}

// writeToString returns a string representation respecting *print-circle*
func writeToString(v *Value) string {
	pc, _ := globalEnv.Get("*print-circle*")
	useCircle := pc != nil && !isNil(pc)
	// Always detect circular structures to prevent infinite loops
	cs := &circleState{seen: make(map[*Value]int), counter: 1}
	findShared(v, cs, make(map[*Value]bool))
	if useCircle && len(cs.seen) > 0 {
		// Print with #n= and #n# labels
		return toStringCircle(v, cs)
	}
	if len(cs.seen) > 0 {
		// Circular but *print-circle* is nil — truncate with ...
		return toStringSafe(v, cs)
	}
	return ToString(v)
}

// toStringSafe prints with ... for circular references (when *print-circle* is nil)
func toStringSafe(v *Value, cs *circleState) string {
	if v == nil {
		return "()"
	}
	switch v.typ {
	case VNil:
		return "()"
	case VPair:
		if _, ok := cs.seen[v]; ok {
			return "..."
		}
		return listToStringSafe(v, cs)
	case VArray:
		if _, ok := cs.seen[v]; ok {
			return "..."
		}
		return arrayToStringSafe(v, cs)
	default:
		return ToString(v)
	}
}

func listToStringSafe(v *Value, cs *circleState) string {
	var b strings.Builder
	b.WriteString("(")
	first := true
	localSeen := make(map[*Value]bool)
	for !isNil(v) {
		if v.typ != VPair {
			b.WriteString(" . ")
			b.WriteString(toStringSafe(v, cs))
			break
		}
		if _, ok := cs.seen[v]; ok && !first {
			b.WriteString("...")
			break
		}
		if localSeen[v] {
			b.WriteString("...")
			break
		}
		localSeen[v] = true
		if !first {
			b.WriteString(" ")
		}
		first = false
		b.WriteString(toStringSafe(v.car, cs))
		v = v.cdr
	}
	b.WriteString(")")
	return b.String()
}

func arrayToStringSafe(v *Value, cs *circleState) string {
	if v.array == nil {
		return "#()"
	}
	var b strings.Builder
	b.WriteString("#(")
	for i, elem := range v.array.elements {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(toStringSafe(elem, cs))
	}
	b.WriteString(")")
	return b.String()
}

// findShared walks the structure and marks values referenced more than once (or circularly)
func findShared(v *Value, cs *circleState, visited map[*Value]bool) {
	if v == nil || v.typ == VNil {
		return
	}
	if v.typ == VPair {
		if visited[v] {
			// Already visited — mark as shared/circular
			if _, exists := cs.seen[v]; !exists {
				cs.seen[v] = cs.counter
				cs.counter++
			}
			return // Don't recurse into already-visited
		}
		visited[v] = true
		findShared(v.car, cs, visited)
		findShared(v.cdr, cs, visited)
	} else if v.typ == VArray && v.array != nil {
		if visited[v] {
			if _, exists := cs.seen[v]; !exists {
				cs.seen[v] = cs.counter
				cs.counter++
			}
			return
		}
		visited[v] = true
		for _, elem := range v.array.elements {
			findShared(elem, cs, visited)
		}
	} else if v.typ == VInstance {
		if visited[v] {
			if _, exists := cs.seen[v]; !exists {
				cs.seen[v] = cs.counter
				cs.counter++
			}
			return
		}
		visited[v] = true
		for _, slot := range v.instSlots {
			findShared(slot, cs, visited)
		}
	}
}

// toStringCircle prints with #n= and #n# syntax for circular/shared references
func toStringCircle(v *Value, cs *circleState) string {
	if v == nil {
		return "()"
	}
	switch v.typ {
	case VNil:
		return "()"
	case VPair:
		if label, ok := cs.seen[v]; ok {
			// Check if already printed (replace the entry with negative to mark)
			if label < 0 {
				return "#" + strconv.Itoa(-label) + "#"
			}
			cs.seen[v] = -label // mark as printed
			prefix := "#" + strconv.Itoa(label) + "="
			return prefix + listToStringCircle(v, cs)
		}
		return listToStringCircle(v, cs)
	case VArray:
		if label, ok := cs.seen[v]; ok {
			if label < 0 {
				return "#" + strconv.Itoa(-label) + "#"
			}
			cs.seen[v] = -label
			prefix := "#" + strconv.Itoa(label) + "="
			return prefix + arrayToStringCircle(v, cs)
		}
		return arrayToStringCircle(v, cs)
	case VInstance:
		if label, ok := cs.seen[v]; ok {
			if label < 0 {
				return "#" + strconv.Itoa(-label) + "#"
			}
			cs.seen[v] = -label
			prefix := "#" + strconv.Itoa(label) + "="
			return prefix + ToString(v)
		}
		return ToString(v)
	default:
		return ToString(v)
	}
}

func listToStringCircle(v *Value, cs *circleState) string {
	var b strings.Builder
	b.WriteString("(")
	for !isNil(v) {
		if v.typ != VPair {
			b.WriteString(" . ")
			b.WriteString(toStringCircle(v, cs))
			break
		}
		// Check cdr for circular reference before recursing
		if !isNil(v.cdr) {
			if label, ok := cs.seen[v.cdr]; ok {
				// cdr is shared — print (a b . #n#) style
				b.WriteString(toStringCircle(v.car, cs))
				b.WriteString(" . ")
				cs.seen[v.cdr] = -absInt(label) // ensure negative for #n#
				b.WriteString(toStringCircle(v.cdr, cs))
				b.WriteString(")")
				return b.String()
			}
		}
		b.WriteString(toStringCircle(v.car, cs))
		v = v.cdr
		if !isNil(v) {
			b.WriteString(" ")
		}
	}
	b.WriteString(")")
	return b.String()
}

func arrayToStringCircle(v *Value, cs *circleState) string {
	if v.array == nil {
		return "#()"
	}
	var b strings.Builder
	b.WriteString("#(")
	for i, elem := range v.array.elements {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(toStringCircle(elem, cs))
	}
	b.WriteString(")")
	return b.String()
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func listToString(v *Value) string {
	if v == nil || v.typ != VPair {
		return "()"
	}
	var b strings.Builder
	b.WriteString("(")
	seen := make(map[*Value]bool)
	for !isNil(v) {
		if seen[v] {
			b.WriteString("...")
			break
		}
		seen[v] = true
		if v.typ != VPair {
			b.WriteString(" . ")
			b.WriteString(toStringWithSeen(v, seen))
			break
		}
		b.WriteString(toStringWithSeen(v.car, seen))
		v = v.cdr
		if !isNil(v) {
			b.WriteString(" ")
		}
	}
	b.WriteString(")")
	return b.String()
}

// toStringWithSeen is like toString but shares a seen map for cycle detection
func toStringWithSeen(v *Value, seen map[*Value]bool) string {
	if v == nil {
		return "()"
	}
	switch v.typ {
	case VNil:
		return "()"
	case VPair:
		if seen[v] {
			return "..."
		}
		// Don't add v to seen here; listToString will do it
		return listToStringShared(v, seen)
	default:
		return ToString(v)
	}
}

func listToStringShared(v *Value, seen map[*Value]bool) string {
	if v == nil || v.typ != VPair {
		return "()"
	}
	var b strings.Builder
	b.WriteString("(")
	for !isNil(v) {
		if seen[v] {
			b.WriteString("...")
			break
		}
		seen[v] = true
		if v.typ != VPair {
			b.WriteString(" . ")
			b.WriteString(toStringWithSeen(v, seen))
			break
		}
		b.WriteString(toStringWithSeen(v.car, seen))
		v = v.cdr
		if !isNil(v) {
			b.WriteString(" ")
		}
	}
	b.WriteString(")")
	return b.String()
}

// -------- File loading --------
func LoadFile(fname string, env *Env) (*Value, error) {
	data, err := os.ReadFile(fname)
	if err != nil {
		return nil, fmt.Errorf("load: %v", err)
	}
	// Save old values of *load-pathname* and *load-truename*
	oldPathname, _ := env.Get("*load-pathname*")
	oldTruename, _ := env.Get("*load-truename*")
	// Set *load-pathname* to the pathname of the file being loaded
	absPath, _ := filepath.Abs(fname)
	env.Set("*load-pathname*", vpathname(parsePathnameString(absPath)))
	// Set *load-truename* to the truename (resolved absolute path)
	env.Set("*load-truename*", vpathname(parsePathnameString(absPath)))
	// Evaluate the file contents
	result, evalErr := EvalString(string(data), env)
	// Restore old values
	if oldPathname != nil {
		env.Set("*load-pathname*", oldPathname)
	} else {
		env.Set("*load-pathname*", vnil())
	}
	if oldTruename != nil {
		env.Set("*load-truename*", oldTruename)
	} else {
		env.Set("*load-truename*", vnil())
	}
	return result, evalErr
}

func builtinMakeThread(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-thread: need a function")
	}
	fn := args[0]
	fnArgs := args[1:]

	tid := atomic.AddInt64(&nextThreadID, 1)
	resultCh := make(chan threadResult, 1)

	threadChannelsMu.Lock()
	threadChannels[tid] = resultCh
	threadChannelsMu.Unlock()

	go func() {
		threadEnv := copyGlobalEnv()
		argList := listFromSlice(fnArgs)
		result, err := Apply(fn, argList, threadEnv)
		resultCh <- threadResult{value: result, err: err}
	}()

	return &Value{typ: VThread, num: float64(tid)}, nil
}

func copyGlobalEnv() *Env {
	env := NewEnv(nil)
	for k, v := range globalEnv.bindings {
		env.bindings[k] = v
	}
	return env
}

func builtinJoinThread(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VThread {
		return nil, fmt.Errorf("join-thread: need a thread")
	}
	tid := int64(args[0].num)
	threadChannelsMu.Lock()
	ch, ok := threadChannels[tid]
	threadChannelsMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("join-thread: no such thread %d", tid)
	}
	tr := <-ch
	if tr.err != nil {
		return nil, tr.err
	}
	threadChannelsMu.Lock()
	delete(threadChannels, tid)
	threadChannelsMu.Unlock()
	return tr.value, nil
}

var nextLockID int64
var atomicCounter int64
var lockMutexMap = make(map[int64]*sync.Mutex)
var lockMapMu sync.Mutex
var condMu sync.Mutex
var condVars = make(map[int64]*sync.Cond)
var nextCondID int64

func builtinMakeLock(args []*Value) (*Value, error) {
	lid := atomic.AddInt64(&nextLockID, 1)
	lockMapMu.Lock()
	lockMutexMap[lid] = &sync.Mutex{}
	lockMapMu.Unlock()
	return &Value{typ: VLock, num: float64(lid)}, nil
}

func builtinLock(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VLock {
		return nil, fmt.Errorf("lock: need a lock object")
	}
	lid := int64(args[0].num)
	lockMapMu.Lock()
	mu, ok := lockMutexMap[lid]
	lockMapMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("lock: invalid lock")
	}
	mu.Lock()
	return vnil(), nil
}

func builtinUnlock(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VLock {
		return nil, fmt.Errorf("unlock: need a lock object")
	}
	lid := int64(args[0].num)
	lockMapMu.Lock()
	mu, ok := lockMutexMap[lid]
	lockMapMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unlock: invalid lock")
	}
	mu.Unlock()
	return vnil(), nil
}

func builtinSleep(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VNum {
		return nil, fmt.Errorf("sleep: need a number of seconds")
	}
	secs := args[0].num
	duration := time.Duration(secs * float64(time.Second))
	time.Sleep(duration)
	return vnil(), nil
}

func builtinValues(args []*Value) (*Value, error) {
	// values returns a VMultiVal wrapping all arguments.
	// Primary value (car) is the first argument, or nil if none.
	v := gcv()
	v.typ = VMultiVal
	v.cdr = vnil()
	if len(args) > 0 {
		v.car = args[0]
		v.cdr = list(args[1:]...)
	}
	return v, nil
}

func builtinValuesList(args []*Value) (*Value, error) {
	// values-list: converts a list to multiple values.
	// (values-list '(a b c)) => values a b c
	if len(args) != 1 {
		return nil, fmt.Errorf("values-list: need exactly 1 argument")
	}
	lst := args[0]
	if isNil(lst) {
		v := gcv()
		v.typ = VMultiVal
		v.car = vnil()
		v.cdr = vnil()
		return v, nil
	}
	v := gcv()
	v.typ = VMultiVal
	v.car = lst.car
	v.cdr = lst.cdr
	return v, nil
}
