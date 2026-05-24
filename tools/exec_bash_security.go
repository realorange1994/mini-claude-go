package tools

import (
	"regexp"
	"strings"
)

// ===========================================================================
// SAFE_ENV_VARS allowlist (upstream bashPermissions.ts lines 378-430)
// ===========================================================================

// safeEnvVars is the allowlist of environment variables that are safe to set
// as VAR=val prefixes on bash commands. Explicitly excluded: PATH, LD_PRELOAD,
// LD_LIBRARY_PATH, DYLD_*, PYTHONPATH, NODE_PATH, GOFLAGS, RUSTFLAGS,
// NODE_OPTIONS, HOME, TMPDIR, SHELL, BASH_ENV.
var safeEnvVars = map[string]bool{
	// Go toolchain
	"GOEXPERIMENT": true, "GOOS": true, "GOARCH": true,
	"CGO_ENABLED": true, "GO111MODULE": true,
	// Rust toolchain
	"RUST_BACKTRACE": true, "RUST_LOG": true,
	// Node.js
	"NODE_ENV": true,
	// Python
	"PYTHONUNBUFFERED": true, "PYTHONDONTWRITEBYTECODE": true,
	"PYTEST_DISABLE_PLUGIN_AUTOLOAD": true, "PYTEST_DEBUG": true,
	// Anthropic SDK
	"ANTHROPIC_API_KEY": true,
	// Locale and language
	"LANG": true, "LANGUAGE": true, "LC_ALL": true, "LC_CTYPE": true,
	"LC_TIME": true, "CHARSET": true,
	// Terminal
	"TERM": true, "COLORTERM": true, "NO_COLOR": true, "FORCE_COLOR": true,
	"TZ": true,
	// Color and formatting
	"LS_COLORS": true, "LSCOLORS": true,
	"GREP_COLOR": true, "GREP_COLORS": true, "GCC_COLORS": true,
	// Date and size formatting
	"TIME_STYLE": true, "BLOCK_SIZE": true, "BLOCKSIZE": true,
}

// unsafeEnvPrefixes are explicitly dangerous env var prefixes.
var unsafeEnvPrefixes = []string{
	"PATH=", "LD_PRELOAD=", "LD_LIBRARY_PATH=", "DYLD_",
	"PYTHONPATH=", "NODE_PATH=", "GOFLAGS=", "RUSTFLAGS=",
	"NODE_OPTIONS=", "HOME=", "TMPDIR=", "SHELL=", "BASH_ENV=",
}

// isUnsafeEnvPrefix checks if a VAR=val assignment prefix is unsafe.
func isUnsafeEnvPrefix(token string) string {
	for _, prefix := range unsafeEnvPrefixes {
		if strings.HasPrefix(strings.ToLower(token), strings.ToLower(prefix)) {
			return prefix
		}
	}
	return ""
}

// isShellOperator reports whether s looks like a shell control operator.
func isShellOperator(s string) bool {
	switch s {
	case "|", "||", "&&", ";", ">", ">>", "<", "<<", ")":
		return true
	}
	return false
}

// checkUnsafeEnvPrefixes scans the command for unsafe environment variable prefixes.
// Returns the unsafe variable name if found, empty string if all safe.
func checkUnsafeEnvPrefixes(cmd string) string {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return ""
	}
	for _, field := range fields {
		// Stop at first shell operator — env vars only appear at the beginning
		if isShellOperator(field) {
			break
		}
		// Check if it looks like VAR=val
		if !strings.Contains(field, "=") {
			break // not an env var assignment, stop checking
		}
		// It's a VAR=val — check if it's unsafe
		if unsafe := isUnsafeEnvPrefix(field); unsafe != "" {
			return field
		}
		// If it's not in the safe list and not in the unsafe list, be conservative
		// and flag it if it's not a recognized safe var
		varName := strings.SplitN(field, "=", 2)[0]
		if !safeEnvVars[varName] {
			// Check unsafe prefixes first (already handled above)
			// For unknown vars, we don't hard-deny, but we could ask.
			// For now, only flag the explicitly unsafe ones.
		}
	}
	return ""
}

// ===========================================================================
// Bash security patterns (upstream bashSecurity.ts 23-step validator chain)
// ===========================================================================

// bashSecurityPattern represents a detected security concern in a bash command.
type bashSecurityPattern struct {
	name     string
	re       *regexp.Regexp
	severity string // "deny" or "ask"
}

var bashSecurityPatterns = []bashSecurityPattern{
	// --- DENY patterns (hard blocks) ---
	// #4 ANSI-C quoting obfuscation: $'...' can encode any character including
	// spaces, quotes, and control characters to bypass regex detection.
	{
		name:     "ANSI-C quoting obfuscation ($'...')",
		re:       regexp.MustCompile(`\$'[^\']*'`),
		severity: "deny",
	},
	// #11 IFS injection: $IFS or ${...IFS...} manipulates the Internal Field
	// Separator to change word splitting behavior.
	{
		name:     "IFS injection ($IFS)",
		re:       regexp.MustCompile(`(?i)\$IFS|\$\{[^}]*IFS[^}]*\}`),
		severity: "deny",
	},
	// #18 Unicode whitespace: non-ASCII whitespace characters that look like
	// spaces but may bypass parser tokenization.
	{
		name:     "Unicode whitespace injection",
		re:       regexp.MustCompile(`[\x{00a0}\x{1680}\x{2000}-\x{200b}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}]`),
		severity: "deny",
	},
	// #17 Carriage return injection: \r outside double quotes can cause the
	// shell to interpret the line differently than the parser expected.
	{
		name:     "Carriage return injection",
		re:       regexp.MustCompile(`\r`),
		severity: "deny",
	},
	// #21 Backslash-escaped operators: \&, \|, \; can bypass parser detection
	// while the shell still interprets them as operators.
	{
		name:     "Backslash-escaped shell operator",
		re:       regexp.MustCompile(`\\[&;|]`),
		severity: "deny",
	},
	// Zsh dangerous module builtins (can load arbitrary modules, perform I/O)
	{
		name:     "Zsh dangerous builtin (zmodload/emulate/sysopen)",
		re:       regexp.MustCompile(`(?i)\b(?:zmodload|emulate|sysopen|sysread|syswrite|sysseek|zpty|ztcp|zsocket|mapfile)\b`),
		severity: "deny",
	},
	// --- ASK patterns (user approval needed) ---
	// #5 Shell metacharacters in quoted arguments: ;, |, & inside quotes can
	// execute subcommands when the quotes are evaluated.
	{
		name:     "Shell metacharacters in quoted context",
		re:       regexp.MustCompile(`["'][^"']*([&;|])[^"']*["']`),
		severity: "ask",
	},
	// #6 Dangerous variable expansion in pipe/redirect context
	{
		name:     "Variable expansion before pipe/redirect",
		re:       regexp.MustCompile(`\$[A-Z_][A-Z_0-9]*\s*[|>]`),
		severity: "ask",
	},
	// #19 Mid-word # comment (Zsh comment syntax can hide commands)
	{
		name:     "Mid-word hash comment",
		re:       regexp.MustCompile(`\w#\w`),
		severity: "ask",
	},
	// #22 Quote/comment boundary desync
	{
		name:     "Quote/comment boundary manipulation",
		re:       regexp.MustCompile(`#\s*['"]|['"]\s*#`),
		severity: "ask",
	},
	// Bare shell executables that are too dangerous to suggest as prefix rules
	// These should never be auto-allowed
	{
		name:     "Dangerous shell executable prefix",
		re:       regexp.MustCompile(`(?i)(?:^|[\s;&|])\b(?:sh|bash|zsh|fish|csh|tcsh|ksh|dash)\b`),
		severity: "ask",
	},
	{
		name:     "Dangerous command modifier prefix",
		re:       regexp.MustCompile(`(?i)(?:^|[\s;&|])\b(?:env|nice|stdbuf|nohup|sudo|doas|pkexec)\b\s`),
		severity: "ask",
	},
	// --- P1 Missing patterns ---
	// #8 Command substitution: $(), ``, <() process substitution, =cmd (zsh)
	// can execute hidden commands that bypass top-level regex checks.
	{
		name:     "Command substitution ($()/backtick/process substitution)",
		re:       regexp.MustCompile(`\$\(|` + "`" + `|<\(|=\w+`),
		severity: "ask",
	},
	// #7 Brace expansion: {a,b} or {1..10} can expand to many arguments,
	// potentially targeting unintended files.
	{
		name:     "Brace expansion",
		re:       regexp.MustCompile(`\{[^}]*\}`),
		severity: "ask",
	},
	// #9 Backslash-escaped whitespace: \ or \\ can be used to obfuscate
	// arguments or bypass word-splitting detection.
	{
		name:     "Backslash-escaped whitespace",
		re:       regexp.MustCompile(`\\[ \t]`),
		severity: "ask",
	},
	// #10 Newline injection: actual newlines (\n) or %0a in commands can
	// cause the shell to execute multiple commands.
	{
		name:     "Newline injection",
		re:       regexp.MustCompile(`(?m)^.*\n.*$`),
		severity: "ask",
	},
	// #11 Control characters: non-printable characters (except \t, \n, \r)
	// can be used for obfuscation.
	{
		name:     "Control character injection",
		re:       regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f]`),
		severity: "ask",
	},
	// #15 Incomplete command: trailing |, ||, &&, ; without continuation
	// may indicate malformed input or injection attempt.
	{
		name:     "Incomplete compound command",
		re:       regexp.MustCompile(`(?:\||\|\||&&|;)\s*$`),
		severity: "ask",
	},
}

// checkBashSecurityPatterns scans a command for known dangerous bash patterns.
func checkBashSecurityPatterns(cmd string) (denyMsgs, askMsgs []string) {
	for _, pat := range bashSecurityPatterns {
		if pat.re.MatchString(cmd) {
			msg := "Bash security: " + pat.name
			if pat.severity == "deny" {
				denyMsgs = append(denyMsgs, msg)
			} else {
				askMsgs = append(askMsgs, msg)
			}
		}
	}
	return denyMsgs, askMsgs
}

// ===========================================================================
// jq security (upstream bashSecurity.ts #2, #3 validateJqCommand)
// ===========================================================================

var jqDenyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bjq\b.*system\s*\(`),       // system() call execution
	regexp.MustCompile(`(?i)\bjq\b.*-[a-z]*f[a-z]*\b`), // -f / --from-file (read filter from file)
	regexp.MustCompile(`(?i)\bjq\b.*--from-file\b`),
	regexp.MustCompile(`(?i)\bjq\b.*--rawfile\b`),       // --rawfile (read file as string)
	regexp.MustCompile(`(?i)\bjq\b.*--slurpfile\b`),     // --slurpfile (read file as JSON)
	regexp.MustCompile(`(?i)\bjq\b.*-L\b`),              // -L / --library-directory (search path)
	regexp.MustCompile(`(?i)\bjq\b.*--library-directory\b`),
}

var jqAskPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bjq\b.*env\[`),   // env[] access (environment variables)
	regexp.MustCompile(`(?i)\bjq\b.*\$ENV\[`), // $ENV[] access
	regexp.MustCompile(`(?i)\bjq\b.*input_filename`), // input_filename (file path leak)
}

// checkJqSecurity validates jq commands against security patterns.
// Returns a message if dangerous, empty if safe.
func checkJqSecurity(cmd string) string {
	lower := strings.ToLower(cmd)
	if !strings.Contains(lower, "jq ") && !strings.HasSuffix(lower, "jq") {
		return ""
	}
	for _, re := range jqDenyPatterns {
		if re.MatchString(lower) {
			return "jq security: dangerous jq operation detected"
		}
	}
	for _, re := range jqAskPatterns {
		if re.MatchString(lower) {
			return "jq security: jq accessing external data sources"
		}
	}
	return ""
}

// ===========================================================================
// sed security (upstream sedValidation.ts + sedEditParser.ts)
// ===========================================================================
// Upstream uses a two-layer approach:
//   Pattern 1 (line printing): sed -n '1p;2,5p' — STRICT allowlist for p, Np, N,Mp
//   Pattern 2 (substitution):  sed 's/foo/bar/g' — STRICT allowlist for flags gpimIM[1-9]
// Defense-in-depth: both patterns must also pass containsDangerousOperations denylist.

// isPrintCommand checks if a single sed command is a valid print command.
// STRICT ALLOWLIST — only these exact forms:
//   p       (print all)
//   Np      (print line N)
//   N,Mp    (print lines N through M)
// Matches upstream isPrintCommand: /^(?:\d+|\d+,\d+)?p$/
func isPrintCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false
	}
	// Allow optional digits or digit,digit range before 'p'
	if cmd[len(cmd)-1] != 'p' {
		return false
	}
	prefix := cmd[:len(cmd)-1]
	if prefix == "" {
		return true // just 'p'
	}
	// Check for N or N,M pattern (digits only, with optional comma)
	if commaIdx := strings.Index(prefix, ","); commaIdx >= 0 {
		before := prefix[:commaIdx]
		after := prefix[commaIdx+1:]
		if before == "" || after == "" {
			return false
		}
		for _, c := range before {
			if c < '0' || c > '9' {
				return false
			}
		}
		for _, c := range after {
			if c < '0' || c > '9' {
				return false
			}
		}
		return true
	}
	// Single line number
	for _, c := range prefix {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// isLinePrintingCommand checks Pattern 1: sed -n with print expressions only.
// Allows: sed -n '1p', sed -n '1,5p', sed -n '1p;2p;3p' with optional -E, -r, -z flags.
// File arguments are ALLOWED for this pattern (read-only, no file writes).
func isLinePrintingCommand(cmd string, expressions []string) bool {
	// Must have at least one expression
	if len(expressions) == 0 {
		return false
	}
	// Validate flags: only allow -n, --quiet, --silent, -E, -r, -z, --posix, --regexp-extended
	if !validateSedFlags(cmd, map[string]bool{
		"-n": true, "--quiet": true, "--silent": true,
		"-E": true, "--regexp-extended": true, "-r": true,
		"-z": true, "--zero-terminated": true, "--posix": true,
	}) {
		return false
	}
	// Check -n flag is present
	if !hasSedNFlag(cmd) {
		return false
	}
	// All expressions must be print commands (semicolons allowed for separating print commands)
	for _, expr := range expressions {
		for _, part := range strings.Split(expr, ";") {
			if !isPrintCommand(strings.TrimSpace(part)) {
				return false
			}
		}
	}
	return true
}

// isSubstitutionCommand checks Pattern 2: sed 's/pattern/replacement/flags'.
// Strict: only / delimiter, flags only gpimIM[1-9].
// When allowFileWrites is false (default), rejects file arguments and -i flag.
func isSubstitutionCommand(cmd string, expressions []string, hasFileArgs bool, allowFileWrites bool) bool {
	if !allowFileWrites && hasFileArgs {
		return false
	}
	// Validate flags
	allowedSubstFlags := map[string]bool{
		"-E": true, "--regexp-extended": true, "-r": true, "--posix": true,
	}
	if allowFileWrites {
		allowedSubstFlags["-i"] = true
		allowedSubstFlags["--in-place"] = true
	}
	if !validateSedFlags(cmd, allowedSubstFlags) {
		return false
	}
	// Must have exactly one expression
	if len(expressions) != 1 {
		return false
	}
	expr := strings.TrimSpace(expressions[0])
	// Must start with 's' (substitution command)
	if len(expr) == 0 || expr[0] != 's' {
		return false
	}
	// Parse s<delim>...<delim>...<delim><flags>
	// sed allows any character except backslash and newline as delimiter,
	// but we only allow / for strict mode.
	rest := expr[1:]
	if len(rest) == 0 {
		return false
	}
	delim := rest[0]
	if delim == '\\' || delim == '\n' {
		return false
	}
	// Find exactly 2 more occurrences of the delimiter (pattern + replacement + flags)
	delimiterCount := 0
	lastDelimIdx := -1
	for i := 1; i < len(rest); i++ {
		if rest[i] == '\\' {
			i++ // skip escaped character
			continue
		}
		if rest[i] == delim {
			delimiterCount++
			lastDelimIdx = i
		}
	}
	// Must have exactly 2 delimiters (3 total with the first one)
	if delimiterCount != 2 || lastDelimIdx < 0 {
		return false
	}
	// Extract flags after the last delimiter
	exprFlags := rest[lastDelimIdx+1:]
	// Validate flags: only gpimIM and optionally one digit 1-9
	validFlags := map[byte]bool{
		'g': true, 'p': true, 'i': true, 'I': true, 'm': true, 'M': true,
	}
	digitSeen := false
	for i := 0; i < len(exprFlags); i++ {
		c := exprFlags[i]
		if c >= '1' && c <= '9' {
			if digitSeen {
				return false // only one digit allowed
			}
			digitSeen = true
			continue
		}
		if !validFlags[c] {
			return false
		}
	}
	return true
}

// validateSedFlags checks that all flags in the command are in the allowed set.
// Handles both single flags (-E) and combined flags (-nE).
func validateSedFlags(cmd string, allowed map[string]bool) bool {
	fields := strings.Fields(cmd)
	for _, f := range fields {
		if !strings.HasPrefix(f, "-") || f == "--" {
			continue
		}
		// Long flags
		if strings.HasPrefix(f, "--") {
			if !allowed[f] {
				return false
			}
			continue
		}
		// Short flags: combined like -nEr
		if len(f) > 2 {
			for _, c := range f[1:] {
				single := "-" + string(c)
				if !allowed[single] {
					return false
				}
			}
			continue
		}
		// Single short flag
		if !allowed[f] {
			return false
		}
	}
	return true
}

// hasSedNFlag checks if the sed command has the -n flag (or --quiet/--silent).
func hasSedNFlag(cmd string) bool {
	fields := strings.Fields(cmd)
	for _, f := range fields {
		if f == "-n" || f == "--quiet" || f == "--silent" {
			return true
		}
		// Check combined flags like -En
		if strings.HasPrefix(f, "-") && !strings.HasPrefix(f, "--") && len(f) > 1 {
			for _, c := range f[1:] {
				if c == 'n' {
					return true
				}
			}
		}
	}
	return false
}

// hasFileArgs checks if a sed command has file arguments (not just stdin).
// Uses simple field parsing: after removing 'sed' and flag args, remaining
// non-flag args beyond the first (the expression) are file arguments.
func hasFileArgs(cmd string) bool {
	fields := strings.Fields(cmd)
	if len(fields) == 0 || strings.ToLower(fields[0]) != "sed" {
		return false
	}

	argCount := 0
	foundExpression := false
	for _, f := range fields[1:] {
		// Handle -e and --expression
		if f == "-e" || f == "--expression" {
			// Next arg is the expression, skip it
			foundExpression = true
			continue
		}
		if strings.HasPrefix(f, "-e=") || strings.HasPrefix(f, "--expression=") {
			foundExpression = true
			continue
		}
		// Skip flags
		if strings.HasPrefix(f, "-") {
			continue
		}
		// Non-flag argument
		if !foundExpression {
			// First non-flag is the sed expression
			foundExpression = true
			continue
		}
		// Additional non-flag = file argument
		argCount++
	}
	return argCount > 0
}

// extractSedExpressions extracts sed expressions from a command string.
// Handles -e expressions, standalone expressions, and --expression=value format.
// Returns empty slice if the command is not a valid sed command.
func extractSedExpressions(cmd string) []string {
	// Check it starts with 'sed '
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return nil
	}
	if strings.ToLower(fields[0]) != "sed" {
		return nil
	}

	// Reject dangerous flag combinations: -ew, -eW, -ee, -we
	for i, f := range fields {
		if strings.HasPrefix(f, "-e") {
			// Check combined -e with dangerous chars
			if len(f) > 2 {
				for _, c := range f[2:] {
					if c == 'w' || c == 'W' || c == 'e' {
						return nil // dangerous flag combo like -ew
					}
				}
			}
		}
		// Check -w or -W followed by expression
		if f == "-w" || f == "-W" {
			if i+1 < len(fields) {
				return nil // -w flag (write to file)
			}
		}
	}

	var expressions []string
	foundEFlag := false

	for _, f := range fields[1:] {
		// Handle -e followed by expression
		if f == "-e" || f == "--expression" {
			foundEFlag = true
			continue
		}
		// Handle --expression=value, -e=value
		if strings.HasPrefix(f, "--expression=") {
			foundEFlag = true
			expressions = append(expressions, f[len("--expression="):])
			continue
		}
		if strings.HasPrefix(f, "-e=") {
			foundEFlag = true
			expressions = append(expressions, f[len("-e="):])
			continue
		}
		// Skip other flags
		if strings.HasPrefix(f, "-") {
			continue
		}
		// Non-flag argument
		if !foundEFlag {
			// First non-flag is the sed expression
			expressions = append(expressions, f)
			foundEFlag = true
			continue
		}
		// If we already have -e, remaining non-flag args are filenames
		break
	}

	return expressions
}

// containsDangerousOperations checks if a sed expression contains dangerous
// operations. This is a defense-in-depth denylist that runs AFTER the allowlist.
// Conservative: when in doubt, treat as unsafe.
func containsDangerousOperations(expression string) bool {
	cmd := strings.TrimSpace(expression)
	if cmd == "" {
		return false
	}
	// Reject non-ASCII characters (Unicode homoglyphs, combining chars)
	for _, c := range cmd {
		if c > 127 {
			return true
		}
	}
	// Reject curly braces (blocks)
	if strings.ContainsRune(cmd, '{') || strings.ContainsRune(cmd, '}') {
		return true
	}
	// Reject newlines
	if strings.ContainsRune(cmd, '\n') {
		return true
	}
	// Reject comments (# not after s command — delimiter check)
	hashIdx := strings.Index(cmd, "#")
	if hashIdx != -1 && !(hashIdx > 0 && cmd[hashIdx-1] == 's') {
		return true
	}
	// Reject negation (! at start or after address)
	if cmd[0] == '!' || strings.Contains(cmd, "!") {
		return true
	}
	// Reject tilde in GNU step address
	if strings.ContainsRune(cmd, '~') {
		return true
	}
	// Reject comma at start (shorthand for 1,$)
	if cmd[0] == ',' {
		return true
	}
	// Reject backslash tricks: s\ (backslash delimiter), \# etc
	if strings.HasPrefix(cmd, "s\\") {
		return true
	}
	if strings.Contains(cmd, "\\#") || strings.Contains(cmd, "\\|") ||
		strings.Contains(cmd, "\\%") || strings.Contains(cmd, "\\@") {
		return true
	}
	// Reject escaped slashes followed by w/W
	if strings.Contains(cmd, "\\/") {
		for _, c := range "wW" {
			if strings.ContainsRune(cmd, c) {
				return true
			}
		}
	}
	// Reject w/W/e/E commands at start or after line numbers
	wWEERe := sedWriteExecRe
	if wWEERe.MatchString(cmd) {
		return true
	}
	// Reject y command (transliteration — complex)
	if cmd[0] == 'y' && len(cmd) > 1 {
		return true
	}
	// Reject substitution with w or e flag
	if cmd[0] == 's' {
		// Find flags after the 3rd delimiter
		rest := cmd[1:]
		if len(rest) > 0 {
			delim := rest[0]
			count := 0
			flagsStart := -1
			escaped := false
			for i := 0; i < len(rest); i++ {
				if escaped {
					escaped = false
					continue
				}
				if rest[i] == '\\' {
					escaped = true
					continue
				}
				if rest[i] == delim {
					count++
					if count == 3 {
						flagsStart = i
						break
					}
				}
			}
			if flagsStart >= 0 {
				flags := rest[flagsStart+1:]
				for _, c := range flags {
					if c == 'w' || c == 'e' || c == 'W' || c == 'E' {
						return true
					}
				}
			}
		}
	}
	// Reject suspicious patterns that look like write/execute
	if sedWriteInContextRe.MatchString(cmd) {
		return true
	}
	return false
}

var sedWriteExecRe = regexp.MustCompile(
	`^(?:[wWeE]\s*\S+|\d+\s*[wWeE]|\$[ \t]+[wWeE]|` +
		`/\w+/[IMim]*[ \t]+[wWeE]|\d+,\d+[ \t]*[wWeE]|` +
		`/\w+/[IMim]*,/\w+/[IMim]*\s*[wWeE]|` +
		`(?:^s.|^\d+\s*e|^\$\s*e|^/\w+/[IMim]*\s*e|^\d+,\d+\s*e|^\d+,\$\s*e))`,
)

var sedWriteInContextRe = regexp.MustCompile(`/[^/]*\s+[wWeE]`)

// checkSedSecurity validates sed commands against the upstream allowlist + denylist.
// Returns empty string if safe, otherwise a security message.
// Pattern 1: sed -n '1p;2,5p' — line printing only
// Pattern 2: sed 's/foo/bar/g' — substitution with safe flags only
// Both must also pass containsDangerousOperations denylist.
func checkSedSecurity(cmd string) string {
	// Quick check: does this look like a sed command?
	fields := strings.Fields(cmd)
	if len(fields) == 0 || strings.ToLower(fields[0]) != "sed" {
		return ""
	}

	// Extract sed expressions
	expressions := extractSedExpressions(cmd)
	if len(expressions) == 0 {
		// No expressions found — could be malformed, let denylist catch it
	}

	// Check if sed has file arguments
	hasFiles := hasFileArgs(cmd)

	// Check Pattern 1 (line printing) and Pattern 2 (substitution)
	isPattern1 := false
	isPattern2 := false

	if len(expressions) > 0 {
		isPattern1 = isLinePrintingCommand(cmd, expressions)
		isPattern2 = isSubstitutionCommand(cmd, expressions, hasFiles, false)
	}

	// Pattern 2 does not allow semicolons (command separators)
	if isPattern2 {
		for _, expr := range expressions {
			if strings.Contains(expr, ";") {
				isPattern2 = false
				break
			}
		}
	}

	// If neither pattern matches, check denylist for defense-in-depth
	if !isPattern1 && !isPattern2 {
		// Run containsDangerousOperations on all expressions
		for _, expr := range expressions {
			if containsDangerousOperations(expr) {
				return "sed security: dangerous operation in sed expression"
			}
		}
		// If no expressions extracted, do a basic denylist scan on the whole command
		if len(expressions) == 0 {
			lower := strings.ToLower(cmd)
			if regexp.MustCompile(`(?i)\bsed\b.*\bw\s+\S+`).MatchString(lower) ||
				regexp.MustCompile(`(?i)\bsed\b.*\bW\s+\S+`).MatchString(lower) {
				return "sed security: write to file (w/W command)"
			}
			if regexp.MustCompile(`(?i)\bsed\b.*\br\s+\S+`).MatchString(lower) {
				return "sed security: read file (r command)"
			}
			if regexp.MustCompile(`(?i)\bsed\b.*s/[^/]*/[^/]*/[a-zA-Z]*e`).MatchString(lower) {
				return "sed security: execute shell command (s///e flag)"
			}
		}
		return ""
	}

	// Defense-in-depth: even if allowlist matches, check denylist
	for _, expr := range expressions {
		if containsDangerousOperations(expr) {
			return "sed security: dangerous operation in sed expression"
		}
	}

	return ""
}

// ===========================================================================
// xargs security (upstream readOnlyValidation.ts xargs config)
// ===========================================================================

// safeXargsFlags is the set of allowed xargs flags with their arg types.
// Arg types: 'none' (no argument), 'replace' (requires '{}' literal),
// 'number' (numeric argument), 'EOF' (requires 'EOF' literal), 'char' (single char).
// NOTE: -i and -e are explicitly excluded because of GNU optional-arg parser differential.
var safeXargsFlags = map[string]string{
	"-I": "replace", // requires literal '{}' as next arg
	"-n": "number",
	"-P": "number",
	"-L": "number",
	"-s": "number",
	"-E": "EOF",    // requires literal 'EOF' as next arg
	"-0": "none",   // null delimiter
	"-t": "none",   // trace/verbose
	"-r": "none",   // no run if empty
	"-x": "none",   // exit if max procs exceeded
	"-d": "char",   // delimiter character
}

// xargsSafeTargets is the allowlist of safe target commands for xargs.
// These commands have been verified to have NO flags that can:
// 1. Write to files (e.g., find's -fprint, sed's -i)
// 2. Execute code (e.g., find's -exec, awk's system(), perl's -e)
// 3. Make network requests
var xargsSafeTargets = map[string]bool{
	"echo":   true,
	"printf": true, // /usr/bin/printf binary, not bash builtin (-v not available)
	"wc":     true,
	"grep":   true,
	"head":   true,
	"tail":   true,
}

// xargsArgTakingFlags are flags that consume arguments (not 'none' type).
// Used to reject bundled short flags that include arg-taking flags.
var xargsArgTakingFlags = map[string]bool{
	"-I": true, "-n": true, "-P": true, "-L": true, "-s": true, "-E": true, "-d": true,
}

// checkXargsSecurity validates xargs commands against security patterns.
// Returns empty string if safe, otherwise a message describing the risk.
func checkXargsSecurity(cmd string) string {
	lower := strings.ToLower(cmd)
	if !strings.Contains(lower, "xargs ") && !strings.HasSuffix(lower, "xargs") {
		return ""
	}

	fields := strings.Fields(cmd)
	xargsIdx := -1
	for i, f := range fields {
		if strings.ToLower(f) == "xargs" {
			xargsIdx = i
			break
		}
	}
	if xargsIdx == -1 {
		return ""
	}

	args := fields[xargsIdx+1:]

	// Find the target command — first non-xargs-flag token.
	// Only validate xargs flags BEFORE the target command.
	// Once we find the target, flags after it belong to the target, not xargs.
	targetIdx := -1
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			// End of xargs flags; target is right after --
			targetIdx = i + 1
			break
		}
		if !strings.HasPrefix(arg, "-") {
			// Non-flag token = target command
			targetIdx = i
			break
		}
		// It's a xargs flag — check arg type to see if it consumes the next token
		argType, ok := safeXargsFlags[arg]
		if ok && argType != "none" {
			i += 2 // skip flag and its argument
			continue
		}
		// Bundled flags like -rt — no arg consumption for 'none' types
		if strings.HasPrefix(arg, "-") && len(arg) > 2 && arg[1] != '-' {
			i++
			continue
		}
		// Long flags
		if strings.HasPrefix(arg, "--") && strings.Contains(arg, "=") {
			i++ // --flag=value doesn't consume next token
			continue
		}
		i++
	}

	// Separate xargs flags (before target) from target command + target flags
	xargsFlags := args
	if targetIdx >= 0 {
		xargsFlags = args[:targetIdx]
	}

	// Step 1: Check for dangerous flags (-i, -e with GNU optional-arg parser differential)
	for _, arg := range xargsFlags {
		if arg == "-i" {
			return "xargs security: -i flag has GNU optional-arg parser differential — use -I {} instead"
		}
		if arg == "-e" {
			return "xargs security: -e flag has GNU optional-arg parser differential — use -E EOF instead"
		}
		// Check bundled flags like -it, -eX
		if strings.HasPrefix(arg, "-") && len(arg) > 2 && arg[1] != '-' && arg[1] != '=' {
			for _, c := range arg[1:] {
				if c == 'i' {
					return "xargs security: bundled -i flag has GNU optional-arg parser differential"
				}
				if c == 'e' {
					return "xargs security: bundled -e flag has GNU optional-arg parser differential"
				}
			}
		}
	}

	// Step 2: Reject bundled flags containing arg-taking flags.
	for _, arg := range xargsFlags {
		if strings.HasPrefix(arg, "-") && len(arg) > 2 && arg[1] != '-' && arg[1] != '=' {
			hasArgFlag := false
			allNone := true
			for _, c := range arg[1:] {
				shortFlag := "-" + string(c)
				if safeXargsFlags[shortFlag] == "" {
					return "xargs security: unrecognized flag " + shortFlag
				}
				if xargsArgTakingFlags[shortFlag] {
					hasArgFlag = true
				}
				if safeXargsFlags[shortFlag] != "none" {
					allNone = false
				}
			}
			if hasArgFlag && !allNone {
				return "xargs security: bundled flags with arg-taking flags must be separated"
			}
		}
	}

	// Step 3: Validate that all xargs flags are in the safe allowlist
	for _, arg := range xargsFlags {
		if !strings.HasPrefix(arg, "-") {
			continue // skip consumed arguments (like {} for -I)
		}
		if arg == "--" {
			continue
		}
		// Bundled short flags (already validated in step 2)
		if strings.HasPrefix(arg, "-") && len(arg) > 2 && arg[1] != '-' {
			continue
		}
		// Long flags
		if strings.HasPrefix(arg, "--") {
			safeLongFlags := map[string]bool{
				"--null": true, "--delimiter": true, "--max-args": true,
				"--max-procs": true, "--max-lines": true, "--max-chars": true,
				"--eof": true, "--verbose": true, "--no-run-if-empty": true,
				"--exit": true,
			}
			flagPart := arg
			if strings.Contains(arg, "=") {
				flagPart = arg[:strings.Index(arg, "=")]
			}
			if !safeLongFlags[flagPart] {
				return "xargs security: unrecognized long flag: " + flagPart
			}
			continue
		}
		// Single short flag
		if _, ok := safeXargsFlags[arg]; !ok {
			return "xargs security: unrecognized flag: " + arg
		}
	}

	// Step 4: Validate target command is in safe allowlist
	if targetIdx >= 0 && targetIdx < len(args) {
		targetCmd := args[targetIdx]
		if strings.ToLower(targetCmd) == "--" && targetIdx+1 < len(args) {
			targetCmd = args[targetIdx+1]
		}
		if !xargsSafeTargets[strings.ToLower(targetCmd)] {
			return "xargs security: target command '" + targetCmd + "' is not in safe allowlist"
		}
	}

	return ""
}

// ===========================================================================
// fd/fdfind security (upstream EXTERNAL_READONLY_COMMANDS)
// ===========================================================================

var safeFdFlags = map[string]bool{
	"-e": true, "-E": true, "-d": true, "-t": true,
	"-S": true, "-0": true, "-H": true,
	"-a": true, "-s": true, "-i": true, "-u": true,
	"--type": true, "--size": true, "--depth": true,
	"--owner": true, "--changed-within": true, "--changed-before": true,
	"--ignore-vcs": true, "--follow": true, "--regex": true,
	"--fixed-strings": true,
	// NOTE: -l intentionally excluded — upstream excludes it because fd -l
	// internally executes `ls` as a subprocess, creating PATH hijack risk.
}

var fdDangerousFlags = map[string]bool{
	"-x": true, "--exec": true,
	"-X": true, "--exec-batch": true,
	"-l": true, // PATH hijack risk — internally executes `ls` as subprocess
}

// checkFdSecurity validates fd/fdfind commands against security patterns.
func checkFdSecurity(cmd string) string {
	lower := strings.ToLower(cmd)
	if !strings.Contains(lower, "fd ") && !strings.Contains(lower, "fdfind ") {
		return ""
	}
	fields := strings.Fields(cmd)
	for _, f := range fields {
		if fdDangerousFlags[strings.ToLower(f)] {
			return "fd security: --exec/-x flag executes arbitrary commands"
		}
	}
	return ""
}

// ===========================================================================
// ripgrep security (upstream RIPGREP_READ_ONLY_COMMANDS)
// ===========================================================================

var rgDangerousFlags = map[string]bool{
	"--pre": true, "--pre-glob": true,
	"--search-zip": true,
}

// checkRgSecurity validates ripgrep commands against security patterns.
func checkRgSecurity(cmd string) string {
	lower := strings.ToLower(cmd)
	if !strings.Contains(lower, "rg ") && !strings.Contains(lower, " ripgrep ") {
		return ""
	}
	fields := strings.Fields(cmd)
	for _, f := range fields {
		if rgDangerousFlags[strings.ToLower(f)] {
			return "ripgrep security: --pre/--search-zip flag has security implications"
		}
	}
	return ""
}

// ===========================================================================
// gh CLI security (upstream GH_READ_ONLY_COMMANDS)
// ===========================================================================

var ghReadOnlySubs = map[string]bool{
	"issue list": true, "issue view": true,
	"pr list": true, "pr view": true, "pr diff": true, "pr commits": true, "pr checks": true,
	"repo view": true, "repo list": true,
	"run list": true, "run view": true,
	"status": true,
}

var ghDangerousSubs = map[string]bool{
	"auth": true, "secret": true, "variable": true,
	"ssh-key": true,
}

// checkGhSecurity validates gh CLI commands against security patterns.
// Includes network exfiltration detection (--repo= form, URL form, SSH form).
func checkGhSecurity(cmd string) string {
	lower := strings.ToLower(cmd)
	if !strings.Contains(lower, "gh ") && !strings.HasSuffix(lower, "gh") {
		return ""
	}

	// Network exfiltration: --repo=evil.com/owner/repo (equals-attached form)
	if ghRepoExfil.MatchString(lower) {
		return "gh security: --repo= form can redirect requests to external host"
	}
	// Network exfiltration: https:// or git@ URL forms
	if ghURLExfil.MatchString(lower) {
		return "gh security: URL form can redirect to external host"
	}
	// Check for 3-segment format HOST/OWNER/REPO (exfil vector)
	if ghHostExfil.MatchString(lower) {
		return "gh security: HOST/OWNER/REPO format can exfiltrate data"
	}
	// Extract subcommand
	fields := strings.Fields(lower)
	for i, f := range fields {
		if f == "gh" && i+1 < len(fields) {
			sub := strings.Join(fields[i+1:], " ")
			// Check for dangerous subcommands
			for ds := range ghDangerousSubs {
				if strings.HasPrefix(sub, ds) {
					return "gh security: gh " + ds + " requires approval"
				}
			}
			// Check for gh api without --method GET
			if strings.HasPrefix(sub, "api") && !strings.Contains(sub, "--method get") {
				return "gh security: gh api requires --method GET for read-only"
			}
			// gist create/edit/delete are dangerous
			if strings.HasPrefix(sub, "gist") {
				for _, dangerous := range []string{"gist create", "gist edit", "gist delete"} {
					if strings.HasPrefix(sub, dangerous) {
						return "gh security: gh " + dangerous + " requires approval"
					}
				}
			}
			break
		}
	}
	return ""
}

// ghRepoExfil detects --repo=evil.com/owner/repo (equals-attached form).
var ghRepoExfil = regexp.MustCompile(`(?i)--repo=\S+\.\S+/`)

// ghURLExfil detects https:// or git@ URL forms in gh commands.
var ghURLExfil = regexp.MustCompile(`(?i)(?:https?://|git@)`)

// ghHostExfil detects HOST/OWNER/REPO three-segment format.
var ghHostExfil = regexp.MustCompile(`\bgh\s+.*[a-z]+\.[a-z]+/[a-z]+/[a-z]+`)

// ===========================================================================
// git command callbacks (upstream additionalCommandIsDangerousCallback)
// ===========================================================================

// gitDangerousArgs maps git subcommands to functions that check for dangerous args.
// Upstream bashPermissions.ts has additionalCommandIsDangerousCallback for git
// branch, tag, reflog, remote — each with subcommand-specific logic.
var gitDangerousArgs = []struct {
	sub    string
	check  func(fields []string) string
}{
	{
		"branch",
		func(fields []string) string {
			// Block positional args (branch names) — can create/delete branches
			// Safe flags: -a, -r, -v, -vv, --list, --merged, --no-merged,
			// --contains, --no-contains, --sort, --format, --points-at,
			// --no-color, --abbrev, -c (copy), -m (move) are write ops
			safeGitBranchFlags := map[string]bool{
				"-a": true, "-r": true, "-v": true, "-vv": true,
				"--list": true, "--merged": true, "--no-merged": true,
				"--contains": true, "--no-contains": true,
				"--sort": true, "--format": true, "--points-at": true,
				"--no-color": true, "--abbrev": true,
				"--verbose": true, "--quiet": true, "--all": true,
				"--remotes": true, "--show-current": true,
			}
			for _, f := range fields {
				if !strings.HasPrefix(f, "-") {
					// Positional arg — could be a branch name being created
					return "git security: positional args to 'git branch' can create/modify branches"
				}
				if !safeGitBranchFlags[strings.ToLower(f)] {
					// Check if flag has a value (e.g., --sort=name)
					if !strings.Contains(f, "=") {
						return "git security: unrecognized flag to 'git branch': " + f
					}
				}
			}
			return ""
		},
	},
	{
		"tag",
		func(fields []string) string {
			// Block tag creation/deletion — only allow -l/--list
			safeGitTagFlags := map[string]bool{
				"-l": true, "--list": true,
				"-n": true, "--contains": true, "--no-contains": true,
				"--merged": true, "--no-merged": true,
				"--sort": true, "--format": true,
				"--no-color": true,
			}
			for _, f := range fields {
				if !strings.HasPrefix(f, "-") {
					return "git security: positional args to 'git tag' can create/delete tags"
				}
				if !safeGitTagFlags[strings.ToLower(f)] {
					if !strings.Contains(f, "=") {
						return "git security: unrecognized flag to 'git tag': " + f
					}
				}
			}
			return ""
		},
	},
	{
		"reflog",
		func(fields []string) string {
			// Block expire/delete — only allow show/exist
			if len(fields) > 0 {
				sub := strings.ToLower(fields[0])
				if sub == "expire" || sub == "delete" {
					return "git security: 'git reflog expire/delete' destroys reflog history"
				}
			}
			return ""
		},
	},
	{
		"remote",
		func(fields []string) string {
			// Only allow -v (verbose listing)
			if len(fields) == 0 {
				return "git security: 'git remote' without -v can modify remotes"
			}
			for _, f := range fields {
				lf := strings.ToLower(f)
				if lf != "-v" && lf != "--verbose" {
					return "git security: 'git remote' with non -v flags can modify remotes"
				}
			}
			return ""
		},
	},
}

// checkGitSecurity validates git commands against subcommand-specific callbacks.
// Returns a message if dangerous, empty if safe.
// Upstream: additionalCommandIsDangerousCallback in bashPermissions.ts.
func checkGitSecurity(cmd string) string {
	lower := strings.ToLower(cmd)
	if !strings.Contains(lower, "git ") && !strings.HasSuffix(lower, "git") {
		return ""
	}
	fields := strings.Fields(lower)
	for i, f := range fields {
		if f == "git" && i+1 < len(fields) {
			// Skip global flags until we find the subcommand.
			// Some global flags take arguments (e.g., -C <path>, --git-dir <path>)
			// that we must also skip.
			gitGlobalFlagsWithArgs := map[string]bool{
				"-c": true,
				"-C": true,
				"--config-env": true,
				"--exec-path": true,
				"--git-dir": true,
				"--work-tree": true,
				"--namespace": true,
				"--super-prefix": true,
			}
			gitSubIdx := i + 1
			for gitSubIdx < len(fields) {
				fld := fields[gitSubIdx]
				if !strings.HasPrefix(fld, "-") {
					break // Found the subcommand
				}
				// Check if this flag takes an argument (e.g., -C <path>)
				// Flags with = form (e.g., -C=/path) don't consume next token
				if strings.Contains(fld, "=") {
					gitSubIdx++
					continue
				}
				if gitGlobalFlagsWithArgs[strings.ToLower(fld)] {
					gitSubIdx += 2 // Skip flag and its value
					continue
				}
				// -C and --bare don't take separate args but -C does
				if strings.ToLower(fld) == "-c" {
					gitSubIdx += 2 // -c key=value
					continue
				}
				gitSubIdx++
			}
			if gitSubIdx >= len(fields) {
				return "" // Just "git -C /path" or similar, no subcommand
			}
			sub := fields[gitSubIdx]
			// Check against dangerous args callbacks
			for _, da := range gitDangerousArgs {
				if sub == da.sub {
					remainingFields := fields[gitSubIdx+1:]
					if msg := da.check(remainingFields); msg != "" {
						return msg
					}
					break
				}
			}
			break
		}
	}
	return ""
}

// ===========================================================================
// Docker security (upstream DOCKER_READ_ONLY_COMMANDS)
// ===========================================================================

var dockerReadOnlySubs = map[string]bool{
	"logs": true, "inspect": true, "ps": true, "images": true,
	"info": true, "version": true, "stats": true, "events": true,
	"history": true, "top": true, "port": true, "diff": true,
}

var dockerDangerousSubs = map[string]bool{
	"rm": true, "rmi": true, "kill": true, "stop": true,
	"pause": true, "unpause": true, "restart": true,
	"run": true, "create": true, "start": true,
	"exec": true, "cp": true, "commit": true, "build": true,
	"load": true, "import": true, "push": true, "pull": true,
	"tag": true, "rename": true, "update": true,
	"network create": true, "network connect": true,
	"network disconnect": true, "network rm": true,
	"volume create": true, "volume rm": true,
}

// checkDockerSecurity validates docker commands against security patterns.
// Returns PermissionResult for read-only (allow), dangerous (ask/deny).
// Returns nil if command is not a docker command.
func checkDockerSecurity(cmd string) *PermissionResult {
	lower := strings.ToLower(cmd)
	if !strings.Contains(lower, "docker ") && !strings.HasSuffix(lower, "docker") {
		return nil
	}
	fields := strings.Fields(lower)
	for i, f := range fields {
		if f == "docker" && i+1 < len(fields) {
			sub := strings.Join(fields[i+1:], " ")
			// Check read-only first
			for rs := range dockerReadOnlySubs {
				if strings.HasPrefix(sub, rs) {
					r := PermissionResultAllow()
					return &r
				}
			}
			// Check dangerous
			for ds := range dockerDangerousSubs {
				if strings.HasPrefix(sub, ds) {
					r := PermissionResultAsk("docker: write operation '"+ds+"' requires approval", "tool")
					return &r
				}
			}
			// Check for prune (already in denyRegexps, but add ask for safety)
			if strings.Contains(sub, "prune") {
				r := PermissionResultDeny("docker: prune operations are blocked")
				return &r
			}
			// Unknown docker subcommand — ask
			r := PermissionResultAsk("docker: unrecognized subcommand requires approval", "tool")
			return &r
		}
	}
	return nil
}

// ===========================================================================
// cd compound attack detection (upstream bashPermissions.ts lines 2182-2225)
// ===========================================================================

// checkCdCompoundAttacks detects compound command attacks involving cd.
func checkCdCompoundAttacks(cmd string, subcmds []string) string {
	cdCount := 0
	hasGit := false
	for _, sub := range subcmds {
		sub = strings.TrimSpace(sub)
		if sub == "" {
			continue
		}
		fields := strings.Fields(sub)
		if len(fields) == 0 {
			continue
		}
		base := filepathBase(fields[0])
		if base == "cd" || base == "pushd" || base == "popd" {
			cdCount++
		}
		if base == "git" {
			hasGit = true
		}
	}

	// Multiple cd in compound command => ask
	if cdCount > 1 {
		return "Multiple cd commands in compound command"
	}

	// cd + git compound => ask (bare repo fsmonitor attack)
	if cdCount > 0 && hasGit {
		return "cd + git compound command (bare repository attack vector)"
	}

	return ""
}

// ===========================================================================
// Read-only command validation helpers
// ===========================================================================

// checkCmdReadOnlyWithFlags validates a command with flag-specific allowlist.
// Returns true if the command uses only safe flags.
func checkCmdReadOnlyWithFlags(cmd string, safeFlags map[string]bool, dangerousFlags map[string]bool) bool {
	lower := strings.ToLower(cmd)
	fields := strings.Fields(lower)
	if len(fields) == 0 {
		return false
	}
	for i, f := range fields[1:] {
		if strings.HasPrefix(f, "-") {
			// Check if it's a dangerous flag
			if dangerousFlags != nil && dangerousFlags[strings.ToLower(f)] {
				return false
			}
			// Check if it's a safe flag
			if safeFlags != nil && !safeFlags[strings.ToLower(f)] {
				// For flags with values (like -I {}), also check next arg
				if i+2 < len(fields) && strings.HasPrefix(fields[i+2], "-") {
					return false
				}
			}
		}
	}
	return true
}

// containsCommandSubstitution detects $() or backtick command substitution in
// a command string. Used to block parser differential attacks like
// `$Z--pre=bash` where $() creates a token that bypasses flag validation.
func containsCommandSubstitution(cmd string) bool {
	return strings.Contains(cmd, "$(") || strings.Contains(cmd, "`")
}

// isReadOnlyCommandWithFlags extends isReadOnlyCommand to validate flags for
// commands that have per-flag allowlists (xargs, fd, rg, sort, ps, file, man, help, netstat).
// Also blocks $() token bypass attacks like `$Z--pre=bash` where command substitution
// creates a token that can bypass flag validation parsers.
func isReadOnlyCommandWithFlags(cmd string, inner string) bool {
	// P0: Block command substitution tokens that can bypass flag validation.
	// Upstream: "if we find $() in the command, it might be a parser differential"
	if containsCommandSubstitution(inner) {
		return false
	}
	// First check command-specific flag allowlists (these override IsReadOnlyCommand)
	lower := strings.ToLower(inner)
	fields := strings.Fields(lower)
	if len(fields) == 0 {
		return false
	}
	bin := fields[0]
	switch bin {
	case "xargs":
		// xargs with safe flags only
		return checkXargsSecurity(inner) == ""
	case "fd", "fdfind":
		return checkFdSecurity(inner) == ""
	case "rg", "ripgrep":
		return checkRgSecurity(inner) == ""
	case "jq":
		return checkJqSecurity(inner) == ""
	case "sed":
		return checkSedSecurity(inner) == ""
	}
	return false
}

// ===========================================================================
// QUOTED_NEWLINE security check (upstream bashSecurity.ts #23 validateQuotedNewline)
// ===========================================================================
// Detects: a quoted newline (inside single or double quotes) immediately followed
// by a line starting with '#'. Attack vector: parser differential — bash sees a
// single multi-line command, but line-based permission checkers strip lines
// beginning with '#' as comments. Malicious args can hide on #-prefixed lines.
// Severity: ASK

// validateQuotedNewline checks for quoted newline followed by #-prefixed line.
// Returns a message if found, empty string otherwise.
func validateQuotedNewline(cmd string) string {
	// Fast-path: must contain both newline and '#' characters
	if !strings.Contains(cmd, "\n") || !strings.Contains(cmd, "#") {
		return ""
	}

	inSingleQuote := false
	inDoubleQuote := false
	inQuote := false
	i := 0

	for i < len(cmd) {
		c := cmd[i]

		switch {
		case c == '\\' && !inSingleQuote:
			// Backslash escapes in double quotes and unquoted context
			i += 2
			continue
		case c == '\'' && !inDoubleQuote:
			inSingleQuote = !inSingleQuote
			inQuote = inSingleQuote || inDoubleQuote
			i++
			continue
		case c == '"' && !inSingleQuote:
			inDoubleQuote = !inDoubleQuote
			inQuote = inSingleQuote || inDoubleQuote
			i++
			continue
		case c == '\n' && inQuote:
			// We're inside a quoted region and hit a newline.
			// Find the next line and check if it starts with '#'.
			nextLineStart := i + 1
			for nextLineStart < len(cmd) && (cmd[nextLineStart] == ' ' || cmd[nextLineStart] == '\t') {
				nextLineStart++
			}
			if nextLineStart < len(cmd) && cmd[nextLineStart] == '#' {
				return "Bash security: quoted newline followed by #-prefixed line can hide arguments from line-based permission checks"
			}
		}
		i++
	}
	return ""
}

// ===========================================================================
// PROC_ENVIRON_ACCESS security check (upstream bashSecurity.ts #13 validateProcEnvironAccess)
// ===========================================================================
// Detects: attempts to access /proc/*/environ files, which expose sensitive
// environment variables (API keys, secrets, credentials) of any process.
// Severity: ASK

var procEnvironRe = regexp.MustCompile(`/proc/[^/]+/environ`)

// validateProcEnvironAccess checks for /proc/*/environ access.
func validateProcEnvironAccess(cmd string) string {
	if procEnvironRe.MatchString(cmd) {
		return "Bash security: command accesses /proc/*/environ which could expose sensitive environment variables"
	}
	return ""
}

// ===========================================================================
// GIT_COMMIT_SUBSTITUTION security check (upstream bashSecurity.ts #12 validateGitCommit)
// ===========================================================================
// Detects: command injection patterns within git commit -m "..." messages.
// Specifically: $(), backticks, or ${} inside double-quoted commit messages.
// Severity: ASK

// validateGitCommit checks for command substitution in git commit messages.
func validateGitCommit(cmd string) string {
	lower := strings.ToLower(cmd)
	// Bail early if it doesn't look like git commit
	if !strings.Contains(lower, "git") || !strings.Contains(lower, "commit") || !strings.Contains(lower, "-m") {
		return ""
	}

	// Bail if input contains backslashes — let full validator handle it
	if strings.Contains(cmd, "\\") {
		return ""
	}

	// Extract the commit message content without using backreferences
	// (Go's regexp doesn't support \1). We manually parse the -m argument.
	quoteChar := byte(0)
	msgContent := strings.Builder{}
	remainder := strings.Builder{}
	foundMsg := false

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]

		if !foundMsg {
			// Look for -m followed by quote
			if c == '-' && i+1 < len(cmd) && cmd[i+1] == 'm' {
				// Check it's -m (not -message, -m=x etc.)
				afterM := i + 2
				if afterM < len(cmd) && cmd[afterM] == '=' {
					// -m="message" form
					afterEq := afterM + 1
					if afterEq < len(cmd) && (cmd[afterEq] == '"' || cmd[afterEq] == '\'') {
						quoteChar = cmd[afterEq]
						i = afterEq + 1
						foundMsg = true
						continue
					}
				} else if afterM < len(cmd) && cmd[afterM] == ' ' {
					// -m "message" form — skip spaces to find quote
					j := afterM + 1
					for j < len(cmd) && cmd[j] == ' ' {
						j++
					}
					if j < len(cmd) && (cmd[j] == '"' || cmd[j] == '\'') {
						quoteChar = cmd[j]
						i = j + 1
						foundMsg = true
						continue
					}
				}
			}
			continue
		}

		// We're reading the message content
		if c == quoteChar {
			// End of quoted message
			// Everything after this is remainder
			if i+1 < len(cmd) {
				remainder.WriteString(cmd[i+1:])
			}
			break
		}
		msgContent.WriteByte(c)
	}

	if !foundMsg {
		return ""
	}

	msg := msgContent.String()

	// Check for command substitution in the message
	if strings.Contains(msg, "$(") || strings.Contains(msg, "`") || strings.Contains(msg, "${") {
		return "Bash security: command substitution in git commit message requires approval"
	}

	// Check for shell operator chaining in the remainder
	rem := strings.TrimSpace(remainder.String())
	if rem != "" {
		for _, c := range rem {
			switch c {
			case ';', '|', '&', '(', ')':
				return "Bash security: shell operator chaining after git commit -m requires approval"
			}
		}
		if strings.Contains(rem, "$(") || strings.Contains(rem, "${") {
			return "Bash security: command substitution after git commit -m requires approval"
		}
	}

	return ""
}

// ===========================================================================
// CheckBashPermission — main entry point for bash/shell permission checks
// ===========================================================================

// CheckBashPermission evaluates a bash/shell command against security patterns,
// unsafe env vars, and per-command validation. Returns:
//   - PermissionResultDeny for hard-blocked patterns
//   - PermissionResultAsk for suspicious but not outright-blocked patterns
//   - PermissionResultPassthrough if the command appears safe (fall through to
//     existing checks)
func CheckBashPermission(cmd string) PermissionResult {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	if lower == "" {
		return PermissionResultPassthrough()
	}

	// Step 1: Bash security patterns
	denyMsgs, askMsgs := checkBashSecurityPatterns(lower)
	if len(denyMsgs) > 0 {
		return PermissionResultDeny(strings.Join(denyMsgs, "; "))
	}
	if len(askMsgs) > 0 {
		return PermissionResultAsk(strings.Join(askMsgs, "; "), "tool")
	}

	// Step 2: Unsafe env var prefixes
	if unsafeEnv := checkUnsafeEnvPrefixes(lower); unsafeEnv != "" {
		return PermissionResultAsk("Unsafe environment variable: "+unsafeEnv, "tool")
	}

	// Step 2b: QUOTED_NEWLINE — parser differential attack where a quoted
	// newline followed by a #-prefixed line hides args from line-based checks.
	if msg := validateQuotedNewline(cmd); msg != "" {
		return PermissionResultAsk(msg, "tool")
	}

	// Step 2c: PROC_ENVIRON_ACCESS — accessing /proc/*/environ can leak
	// sensitive environment variables (API keys, credentials) of any process.
	if msg := validateProcEnvironAccess(cmd); msg != "" {
		return PermissionResultAsk(msg, "tool")
	}

	// Step 2d: GIT_COMMIT_SUBSTITUTION — command substitution in git commit
	// messages can execute arbitrary commands disguised as commit text.
	if msg := validateGitCommit(cmd); msg != "" {
		return PermissionResultAsk(msg, "tool")
	}

	// Step 3: Read-only command allowlist (before security checks)
	// Read-only allowlist uses original `cmd` to preserve case-sensitive flags,
	// while security checks use `lower`. For xargs/rg/grep etc, if the command
	// passes the read-only allowlist, security checks are redundant.
	if checkBashReadOnlyCommand(cmd) {
		return PermissionResultAllow()
	}

	// Step 4: Per-command security validation
	// jq
	if msg := checkJqSecurity(lower); msg != "" {
		return PermissionResultAsk(msg, "tool")
	}
	// sed
	if msg := checkSedSecurity(lower); msg != "" {
		return PermissionResultAsk(msg, "tool")
	}
	// xargs (dangerous flags like -i/-e are also caught by read-only allowlist's
	// dangerousFlags map, but this check catches them if they bypass read-only)
	if msg := checkXargsSecurity(lower); msg != "" {
		return PermissionResultAsk(msg, "tool")
	}
	// fd/fdfind (dangerous flags like -x/-X are also caught by read-only allowlist)
	if msg := checkFdSecurity(lower); msg != "" {
		return PermissionResultAsk(msg, "tool")
	}
	// ripgrep (dangerous flags like --pre are also caught by read-only allowlist)
	if msg := checkRgSecurity(lower); msg != "" {
		return PermissionResultAsk(msg, "tool")
	}
	// gh CLI
	if msg := checkGhSecurity(lower); msg != "" {
		return PermissionResultAsk(msg, "tool")
	}
	// git: check read-only allowlist first, then subcommand-specific callbacks
	// Use original `cmd` (not `lower`) to preserve case-sensitive git flags (-M vs -m)
	if bashROIsGitReadOnlyCommand(cmd) {
		return PermissionResultAllow()
	}
	if msg := checkGitSecurity(lower); msg != "" {
		return PermissionResultAsk(msg, "tool")
	}
	// git: unknown subcommand with safe global flags → ask
	if strings.HasPrefix(lower, "git ") {
		return PermissionResultAsk("git: unrecognized subcommand or flags requires approval", "tool")
	}
	// docker (returns full PermissionResult)
	if dockerResult := checkDockerSecurity(lower); dockerResult != nil {
		return *dockerResult
	}

	return PermissionResultPassthrough()
}

// filepathBase is a wrapper around filepath.Base to avoid import in this file.
func filepathBase(path string) string {
	// Handle both Unix and Windows path separators
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		path = path[idx+1:]
	}
	if idx := strings.LastIndex(path, "\\"); idx >= 0 {
		path = path[idx+1:]
	}
	return path
}

// bashMin returns the minimum of two integers.
// Named to avoid shadowing Go 1.21+ built-in min.
func bashMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
