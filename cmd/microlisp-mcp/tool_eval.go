package main

import (
	"encoding/json"
	"fmt"
	"time"

	"miniclaudecode-go/microlisp"
)

func textResult(text string, isError bool) ToolCallResult {
	return ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
		IsError: isError,
	}
}

func limitsDuration(limits microlisp.ResourceLimits) time.Duration {
	if limits.MaxTimeMs <= 0 {
		return 0
	}
	return time.Duration(limits.MaxTimeMs) * time.Millisecond
}

func RegisterEvalTool(s *MCPServer) {
	s.RegisterTool(ToolDef{
		Name:        "lisp_eval",
		Description: "Evaluate Common Lisp CODE / EXPRESSIONS — NOT a command runner. This is a Lisp interpreter, NOT os/exec. Do NOT use this to run system commands like go, python, npm, ls — use lisp_exec for that. Use this to evaluate Lisp code only. State persists between calls. Quick start: expression=\"(+ 1 2)\". Use operation=\"define\" to see function signatures (params, return types). Use operation=\"help\" for topic docs, \"examples\" for code samples, \"skill\" for a complete usage guide. FFI: (ffi \"math.Sqrt\") calls Go stdlib.",
		InputSchema: map[string]any{
			"type":        "object",
			"description": "Evaluates Lisp CODE / EXPRESSIONS via a Lisp interpreter. NOT for running system commands (go, python, npm, shell) — use lisp_exec for that. Do NOT use this to execute CLI tools — it is a Lisp evaluator only.",
			"properties": map[string]any{
				"expression": map[string]any{
					"type":        "string",
					"description": "Lisp expression to evaluate (e.g. (+ 1 2), (car '(1 2 3))). This is Lisp CODE, NOT a shell command. For system commands, use lisp_exec. For source/xref operations: use a PLAIN function name only (e.g. \"car\"). For help/skill: use a topic name like \"arithmetic\" or \"ffi\".",
				},
				"operation": map[string]any{
					"type":        "string",
					"enum":        []string{"eval", "reset", "help", "examples", "eval_file", "lint", "source", "source-list", "xref", "xref-list", "skill", "define"},
					"description": `Action: eval (default, evaluate expression), reset (clear state), help (topic docs), examples (code samples), skill (usage guide — expression="ffi"/"ops"/"xref" or empty for full), define (function signature — params, return types, usage), source (view function source — expression=plain name), source-list (browse indexed functions), xref (call graph — expression=plain name), xref-list (browse all xrefs), eval_file (run .lisp file), lint (check syntax)`,
				},
				"file": map[string]any{
					"type":        "string",
					"description": "File path for operation=eval_file or lint. Required for eval_file. Optional for lint (use either expression or file).",
				},
				"limits": map[string]any{
					"type":        "string",
					"enum":        []string{"default", "strict", "unlimited"},
					"description": "Resource limit profile for safety: 'default' (1M steps, 30s, 256MB heap) for normal use; 'strict' (100K steps, 10s, 64MB heap) for untrusted code; 'unlimited' disables all limits (REPL mode only). Defaults to 'default'.",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Zero-based line offset for source code display, or entry offset for source-list pagination (default: 0).",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Max lines to return for source code display, or max entries for source-list (default: 50).",
				},
			},
			"required": []string{},
		},
	}, handleEval)
}

func handleEval(params json.RawMessage) (ToolCallResult, error) {
	var p struct {
		Operation string `json:"operation"`
		Expression string `json:"expression"`
		File      string `json:"file"`
		Limits    string `json:"limits"`
		Offset    int    `json:"offset"`
		Limit     int    `json:"limit"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return textResult("Invalid params: "+err.Error(), true), nil
	}

	switch p.Operation {
	case "reset":
		ch := make(chan struct{}, 1)
		go func() {
			microlisp.ResetGlobalEnv()
			ch <- struct{}{}
		}()
		select {
		case <-ch:
			return textResult("Lisp interpreter state has been reset. All user-defined variables, functions, macros, and classes have been cleared.", false), nil
		}

	case "help":
		return textResult(lispHelp(p.Expression), false), nil

	case "skill":
		return textResult(lispSkill(p.Expression), false), nil

	case "examples":
		return textResult(lispExamples(p.Expression), false), nil

	case "eval_file":
		if p.File == "" {
			return textResult("Error: file is required for operation=eval_file", true), nil
		}
		limits := selectLimits(p.Limits)
		cancelChan := microlisp.NewCancelChannel()
		limits.CancelChan = cancelChan
		type evalRes struct {
			output string
			err    error
		}
		ch := make(chan evalRes, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					ch <- evalRes{"", fmt.Errorf("panic: %v", r)}
				}
			}()
			output, err := microlisp.SafeLoadFileWithLimits(p.File, limits)
			ch <- evalRes{output, err}
		}()
		dur := limitsDuration(limits)
		if dur > 0 {
			timer := time.NewTimer(dur)
			select {
			case <-timer.C:
				close(cancelChan)
				<-ch
				return textResult("Error: lisp_eval timed out loading file", true), nil
			case r := <-ch:
				timer.Stop()
				if r.err != nil {
					return textResult(fmt.Sprintf("Error: %v", r.err), true), nil
				}
				out := r.output
				if out == "" {
					out = "NIL"
				}
				return textResult(out, false), nil
			}
		}
		r := <-ch
		if r.err != nil {
			return textResult(fmt.Sprintf("Error: %v", r.err), true), nil
		}
		out := r.output
		if out == "" {
			out = "NIL"
		}
		return textResult(out, false), nil

	case "lint":
		limits := selectLimits(p.Limits)
		cancelChan := microlisp.NewCancelChannel()
		limits.CancelChan = cancelChan
		type evalRes struct {
			output string
			err    error
		}
		if p.File != "" {
			ch := make(chan evalRes, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						ch <- evalRes{"", fmt.Errorf("panic: %v", r)}
					}
				}()
				err := microlisp.SafeLintFileWithLimits(p.File, limits)
				if err != nil {
					ch <- evalRes{"", err}
				} else {
					ch <- evalRes{"No syntax errors found.", nil}
				}
			}()
			dur := limitsDuration(limits)
			if dur > 0 {
				timer := time.NewTimer(dur)
				select {
				case <-timer.C:
					close(cancelChan)
					<-ch
					return textResult("Error: lisp_eval timed out during lint", true), nil
				case r := <-ch:
					timer.Stop()
					if r.err != nil {
						return textResult(fmt.Sprintf("Lint error: %v", r.err), true), nil
					}
					return textResult(r.output, false), nil
				}
			}
			r := <-ch
			if r.err != nil {
				return textResult(fmt.Sprintf("Lint error: %v", r.err), true), nil
			}
			return textResult(r.output, false), nil
		}
		if p.Expression == "" {
			return textResult("Error: expression or file is required for operation=lint", true), nil
		}
		ch := make(chan evalRes, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					ch <- evalRes{"", fmt.Errorf("panic: %v", r)}
				}
			}()
			err := microlisp.SafeLintWithLimits(p.Expression, limits)
			if err != nil {
				ch <- evalRes{"", err}
			} else {
				ch <- evalRes{"No syntax errors found.", nil}
			}
		}()
		dur := limitsDuration(limits)
		if dur > 0 {
			timer := time.NewTimer(dur)
			select {
			case <-timer.C:
				close(cancelChan)
				<-ch
				return textResult("Error: lisp_eval timed out during lint", true), nil
			case r := <-ch:
				timer.Stop()
				if r.err != nil {
					return textResult(fmt.Sprintf("Lint error: %v", r.err), true), nil
				}
				return textResult(r.output, false), nil
			}
		}
		r := <-ch
		if r.err != nil {
			return textResult(fmt.Sprintf("Lint error: %v", r.err), true), nil
		}
		return textResult(r.output, false), nil

	case "define":
		if p.Expression == "" {
			return textResult("Error: expression is required for operation=define. Use a plain function name like \"car\" or \"math.Sin\" — NOT a Lisp expression.", true), nil
		}
		return textResult(microlisp.GetDefine(p.Expression), false), nil

	case "source":
		if p.Expression == "" {
			return textResult("Error: expression is required for operation=source. Use a plain function name like \"car\" or \"string-append\" — NOT a Lisp expression like (source 'car).", true), nil
		}
		return textResult(microlisp.GetSource(p.Expression, p.Offset, p.Limit), false), nil

	case "source-list":
		return textResult(microlisp.SourceList(p.Expression, p.Offset, p.Limit), false), nil

	case "xref":
		if p.Expression == "" {
			return textResult("Error: expression is required for operation=xref. Use a plain function name like \"car\" or \"string-append\".", true), nil
		}
		contextLines := 2
		if p.Limit > 0 {
			contextLines = p.Limit
		}
		return textResult(microlisp.GetXRef(p.Expression, contextLines), false), nil

	case "xref-list":
		return textResult(microlisp.XRefList(p.Expression, p.Offset, p.Limit), false), nil

	default: // eval
		if p.Expression == "" {
			return textResult("Error: expression is required. Examples: (+ 1 2) => 3, (car '(1 2 3)) => 1", true), nil
		}
		limits := selectLimits(p.Limits)
		cancelChan := microlisp.NewCancelChannel()
		limits.CancelChan = cancelChan
		type evalRes struct {
			output string
			err    error
		}
		ch := make(chan evalRes, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					ch <- evalRes{"", fmt.Errorf("panic: %v", r)}
				}
			}()
			result, err := microlisp.SafeEvalWithLimits(p.Expression, limits)
			ch <- evalRes{result, err}
		}()
		dur := limitsDuration(limits)
		if dur > 0 {
			timer := time.NewTimer(dur)
			select {
			case <-timer.C:
				close(cancelChan)
				<-ch
				return textResult("Error: lisp_eval timed out evaluating expression", true), nil
			case r := <-ch:
				timer.Stop()
				if r.err != nil {
					return textResult(fmt.Sprintf("Error: %v", r.err), true), nil
				}
				out := r.output
				if out == "" {
					out = "NIL"
				}
				return textResult(out, false), nil
			}
		}
		r := <-ch
		if r.err != nil {
			return textResult(fmt.Sprintf("Error: %v", r.err), true), nil
		}
		out := r.output
		if out == "" {
			out = "NIL"
		}
		return textResult(out, false), nil
	}
}

func selectLimits(profile string) microlisp.ResourceLimits {
	switch profile {
	case "strict":
		return microlisp.StrictLimits()
	case "unlimited":
		return microlisp.UnlimitedLimits()
	default:
		return microlisp.DefaultLimits()
	}
}
