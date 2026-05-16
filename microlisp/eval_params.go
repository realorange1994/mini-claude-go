package microlisp

import "fmt"

func parseParams(v *Value) ([]string, string, []*Value, []*Value, error) {
	if v.typ == VSym {
		return nil, v.str, nil, nil, nil
	}
	var params []string
	var optDefaults []*Value // default exprs for optional params (nil = use NIL)
	var keySpecs []*Value    // (keyword param-name default) for &key params
	seen := make(map[*Value]bool)
	inOptional := false
	inKey := false
	for !isNil(v) {
		// Handle dotted tail: when v becomes a VSym (e.g. (a b . rest)),
		// treat it as &rest parameter
		if v.typ == VSym {
			return params, v.str, optDefaults, keySpecs, nil
		}
		if seen[v] {
			return nil, "", nil, nil, fmt.Errorf("bad lambda parameter list: circular")
		}
		seen[v] = true
		elem := v.car
		// Check for keyword symbols first
		if elem != nil && elem.typ == VSym {
			s := elem.str
			if s == "&rest" || s == "&REST" || s == "&body" || s == "&BODY" {
				if v.cdr == nil || v.cdr.typ != VPair || v.cdr.car == nil || v.cdr.car.typ != VSym {
					return nil, "", nil, nil, fmt.Errorf("bad lambda parameter list: %s requires a symbol", s)
				}
				return params, v.cdr.car.str, optDefaults, keySpecs, nil
			}
			if s == "&optional" || s == "&OPTIONAL" {
				inOptional = true
				inKey = false
				v = v.cdr
				continue
			}
			if s == "&key" || s == "&KEY" {
				inOptional = false
				inKey = true
				v = v.cdr
				continue
			}
			if s == "&allow-other-keys" || s == "&AUX" || s == "&aux" || s == "&whole" || s == "&WHOLE" || s == "&environment" || s == "&ENVIRONMENT" {
				v = v.cdr
				continue
			}
			// Not a keyword, fall through to param handling
		}
		if inKey {
			// &key param
			var keyword, paramName string
			var defaultExpr *Value = vnil()
			if elem != nil && elem.typ == VSym {
				keyword = ":" + elem.str
				paramName = elem.str
			} else if elem != nil && elem.typ == VPair && elem.car != nil && elem.car.typ == VSym {
				// Check if this is ((keyword param) default) or (param default)
				if elem.car.typ == VSym && elem.car.str != "" && elem.car.str[0] == ':' {
					// This is ((:keyword param) default) format
					keyword = elem.car.str // already starts with ':'
					if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil && elem.cdr.car.typ == VSym {
						paramName = elem.cdr.car.str
						if elem.cdr.cdr != nil && elem.cdr.cdr.typ == VPair && elem.cdr.cdr.car != nil {
							defaultExpr = elem.cdr.cdr.car
						}
					}
				} else if elem.car.typ == VPair && elem.car.car != nil && elem.car.car.typ == VSym {
					// This is ((keyword param) default) format where keyword may not start with ':'
					keyword = ":" + elem.car.car.str
					if elem.car.cdr != nil && elem.car.cdr.typ == VPair && elem.car.cdr.car != nil && elem.car.cdr.car.typ == VSym {
						paramName = elem.car.cdr.car.str
					}
					if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil {
						defaultExpr = elem.cdr.car
					}
				} else {
					// This is (param default) format - keyword is :PARAM
					keyword = ":" + elem.car.str
					paramName = elem.car.str
					if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil {
						defaultExpr = elem.cdr.car
					}
				}
			}
			if paramName != "" {
				params = append(params, paramName)
				keySpecs = append(keySpecs, list3(vsym(keyword), vsym(paramName), defaultExpr))
			} else {
			}
			v = v.cdr
			continue
		}
		if inOptional {
			// Optional param: can be symbol or (symbol default)
			if elem != nil && elem.typ == VSym {
				params = append(params, elem.str)
				optDefaults = append(optDefaults, nil)
			} else if elem != nil && elem.typ == VPair && elem.car != nil && elem.car.typ == VSym {
				params = append(params, elem.car.str)
				var defaultExpr *Value = vnil()
				if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil {
					defaultExpr = elem.cdr.car
				}
				optDefaults = append(optDefaults, defaultExpr)
			}
			v = v.cdr
			continue
		}
		// Regular required param
		if elem != nil && elem.typ == VSym {
			params = append(params, elem.str)
		}
		v = v.cdr
	}
	if !isNil(v) && v.typ == VSym {
		return params, v.str, optDefaults, keySpecs, nil
	}
	return params, "", optDefaults, keySpecs, nil
}

func parseMacroParams(v *Value) (params []string, rest string, whole string, envSym string, optDefaults []*Value, keySpecs []*Value, err error) {
	// Check for &whole at the beginning
	if v.typ == VPair && v.car != nil && v.car.typ == VSym && (v.car.str == "&whole" || v.car.str == "&WHOLE") {
		restV := v.cdr
		if !isNil(restV) && restV.typ == VPair && restV.car != nil && restV.car.typ == VSym {
			whole = restV.car.str
			v = restV.cdr
		} else if !isNil(restV) && restV.typ == VSym {
			whole = restV.str
			v = vnil()
		}
	}
	inOptional := false
	inKey := false
	seen := make(map[*Value]bool)
	for !isNil(v) {
		if seen[v] {
			return nil, "", whole, envSym, nil, nil, fmt.Errorf("bad macro parameter list: circular")
		}
		seen[v] = true
		if v.car != nil && v.car.typ == VSym {
			s := v.car.str
			if s == "&rest" || s == "&body" || s == "&REST" || s == "&BODY" {
				restV := v.cdr
				if !isNil(restV) && restV.typ == VPair && restV.car != nil && restV.car.typ == VSym {
					rest = restV.car.str
					v = restV.cdr
				} else if !isNil(restV) && restV.typ == VSym {
					rest = restV.str
					v = vnil()
				} else {
					return nil, "", whole, envSym, nil, nil, fmt.Errorf("macro: need name after %s", s)
				}
				continue
			}
			if s == "&environment" || s == "&ENVIRONMENT" {
				envV := v.cdr
				if !isNil(envV) && envV.typ == VPair && envV.car != nil && envV.car.typ == VSym {
					envSym = envV.car.str
					v = envV.cdr
				} else if !isNil(envV) && envV.typ == VSym {
					envSym = envV.str
					v = vnil()
				}
				continue
			}
			if s == "&optional" || s == "&OPTIONAL" {
				inOptional = true
				inKey = false
				v = v.cdr
				continue
			}
			if s == "&key" || s == "&KEY" {
				inOptional = false
				inKey = true
				v = v.cdr
				continue
			}
			if s == "&allow-other-keys" || s == "&AUX" || s == "&aux" || s == "&whole" || s == "&WHOLE" {
				v = v.cdr
				continue
			}
			if inKey {
				keyword := ":" + s
				params = append(params, s)
				keySpecs = append(keySpecs, list3(vsym(keyword), vsym(s), vnil()))
				v = v.cdr
				continue
			}
			if inOptional {
				params = append(params, s)
				optDefaults = append(optDefaults, nil)
				v = v.cdr
				continue
			}
			params = append(params, s)
		} else if v.car != nil && v.car.typ == VPair {
			car := v.car.car
			if car != nil && car.typ == VSym {
				if inKey {
					var keyword, paramName string
					var defaultExpr *Value = vnil()
					elem := v.car
					if elem.car.typ == VPair && elem.car.car != nil && elem.car.car.typ == VSym {
						keyword = ":" + elem.car.car.str
						if elem.car.cdr != nil && elem.car.cdr.typ == VPair && elem.car.cdr.car != nil && elem.car.cdr.car.typ == VSym {
							paramName = elem.car.cdr.car.str
						}
						if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil {
							defaultExpr = elem.cdr.car
						}
					} else {
						keyword = ":" + elem.car.str
						paramName = elem.car.str
						if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil {
							defaultExpr = elem.cdr.car
						}
					}
					if paramName != "" {
						params = append(params, paramName)
						keySpecs = append(keySpecs, list3(vsym(keyword), vsym(paramName), defaultExpr))
					}
					v = v.cdr
					continue
				}
				if inOptional {
					params = append(params, car.str)
					var defaultExpr *Value = vnil()
					if v.car.cdr != nil && v.car.cdr.typ == VPair && v.car.cdr.car != nil {
						defaultExpr = v.car.cdr.car
					}
					optDefaults = append(optDefaults, defaultExpr)
					v = v.cdr
					continue
				}
				params = append(params, car.str)
			} else {
				break
			}
		} else {
			break
		}
		v = v.cdr
	}
	if !isNil(v) && v.typ == VSym {
		return params, v.str, whole, envSym, optDefaults, keySpecs, nil
	}
	if !isNil(v) {
		return nil, "", whole, envSym, nil, nil, fmt.Errorf("bad macro parameter list")
	}
	return params, rest, whole, envSym, optDefaults, keySpecs, nil
}
