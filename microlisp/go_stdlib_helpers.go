package microlisp

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// Go-side helper functions for stdlib wrappers that the FFI can't handle well.
// These are registered as builtins and provide access to Go stdlib
// functionality that requires package-level variables or complex type handling.

func builtinBase64Encode(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("base64-encode: need a string")
	}
	return vstr(base64.StdEncoding.EncodeToString([]byte(args[0].str))), nil
}

func builtinBase64Decode(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("base64-decode: need a string")
	}
	decoded, err := base64.StdEncoding.DecodeString(args[0].str)
	if err != nil {
		return nil, fmt.Errorf("base64-decode: %v", err)
	}
	return vstr(string(decoded)), nil
}

func builtinMD5(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("md5: need a string")
	}
	hash := md5.Sum([]byte(args[0].str))
	return vstr(hex.EncodeToString(hash[:])), nil
}

func builtinSHA1(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("sha1: need a string")
	}
	hash := sha1.Sum([]byte(args[0].str))
	return vstr(hex.EncodeToString(hash[:])), nil
}

func builtinSHA256(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("sha256: need a string")
	}
	hash := sha256.Sum256([]byte(args[0].str))
	return vstr(hex.EncodeToString(hash[:])), nil
}
