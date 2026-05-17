package microlisp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
func builtinCompileFilePathname(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("compile-file-pathname: need a pathname")
	}
	path := args[0]
	var fileStr string
	if path.typ == VStr {
		fileStr = path.str
	} else if path.typ == VPathname {
		fileStr = pathnameToString(path.pathname)
	} else {
		return nil, fmt.Errorf("compile-file-pathname: need a pathname or string")
	}
	outputPath := fileStr
	if strings.HasSuffix(outputPath, ".lisp") {
		outputPath = outputPath[:len(outputPath)-5] + ".fas"
	} else if strings.HasSuffix(outputPath, ".lsp") {
		outputPath = outputPath[:len(outputPath)-4] + ".fas"
	} else if !strings.HasSuffix(outputPath, ".fas") {
		outputPath += ".fas"
	}
	p := parsePathnameString(outputPath)
	v := gcv()
	v.typ = VPathname
	v.pathname = p
	return v, nil
}

func builtinCompileFile(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("compile-file: need a pathname")
	}
	path := args[0]
	var fileStr string
	if path.typ == VStr {
		fileStr = path.str
	} else if path.typ == VPathname {
		fileStr = pathnameToString(path.pathname)
	} else {
		return nil, fmt.Errorf("compile-file: need a pathname or string")
	}
	// Read and parse the file (MicroLisp has no native code compiler, so we just parse)
	data, err := os.ReadFile(fileStr)
	if err != nil {
		return nil, fmt.Errorf("compile-file: could not read %s: %v", fileStr, err)
	}
	forms, perr := parseAll(string(data))
	if perr != nil {
		return nil, fmt.Errorf("compile-file: parse error in %s: %v", fileStr, perr)
	}
	for !isNil(forms) {
		_, err := Eval(forms.car, globalEnv)
		if err != nil {
			return nil, fmt.Errorf("compile-file: error in %s: %v", fileStr, err)
		}
		forms = forms.cdr
	}
	// Return output pathname (append .fas to input name)
	outputPath := fileStr
	if !strings.HasSuffix(outputPath, ".lisp") && !strings.HasSuffix(outputPath, ".lsp") {
		outputPath += ".fas"
	} else if strings.HasSuffix(outputPath, ".lisp") {
		outputPath = outputPath[:len(outputPath)-5] + ".fas"
	} else {
		outputPath = outputPath[:len(outputPath)-4] + ".fas"
	}
	p := parsePathnameString(outputPath)
	v := gcv()
	v.typ = VPathname
	v.pathname = p
	return v, nil
}
