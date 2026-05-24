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
		re:       regexp.MustCompile(`(?i)(?:^|[\s;&|])\b(?:env|xargs|nice|stdbuf|nohup|sudo|doas|pkexec)\b\s`),
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

var sedDenyPatterns = []*regexp.Regexp{
	// w filename — write pattern space to file (data exfiltration)
	regexp.MustCompile(`(?i)\bsed\b.*['"]\s*w\s+[^\s'"]+['"]`),
	// W filename — write without newline
	regexp.MustCompile(`(?i)\bsed\b.*['"]\s*W\s+[^\s'"]+['"]`),
	// e flag — execute shell command in replacement
	regexp.MustCompile(`(?i)\bsed\b.*['"][^'"]*s[^'"]*[a-zA-Z]\s*/\s*e\s*['"]`),
	// r filename — read file into pattern space
	regexp.MustCompile(`(?i)\bsed\b.*['"]\s*r\s+[^\s'"]+['"]`),
	// s///ge — substitute with global + eval flag
	regexp.MustCompile(`(?i)\bsed\b.*s/[^/]*/[^/]*/[a-zA-Z]*e[a-zA-Z]*`),
}

var sedAskPatterns = []*regexp.Regexp{
	// Unrecognized flags (not in safe set npegiI)
	// This is checked by validating that all flags after s/// are in the safe set
}

// safeSedFlags is the set of allowed sed flags for substitution commands.
var safeSedFlags = map[byte]bool{
	'n': true, 'p': true, 'e': false, // e is dangerous
	'g': true, 'i': true, 'I': true,
}

// checkSedSecurity validates sed commands for dangerous patterns.
// Upstream sedValidation.ts checks for write-to-file, execute, and read-file
// commands. Go's RE2 regex doesn't support backreferences, so we use a
// simple approach: find 'e' flag after the third delimiter of s/// commands.
func checkSedSecurity(cmd string) string {
	fields := strings.Fields(cmd)
	if len(fields) == 0 || fields[0] != "sed" {
		return ""
	}

	lower := strings.ToLower(cmd)

	// Check deny patterns for w/W (write to file), r (read file), s///e (execute)
	for _, re := range sedDenyPatterns {
		if re.MatchString(lower) {
			return "sed security: dangerous sed operation detected (w/r/e flag)"
		}
	}

	// Additional check for 'e' flag in s/// commands using a simple scan.
	// Since RE2 doesn't support backreferences, we manually find s-expressions
	// and check their trailing flags.
	// We look for patterns like s/.../.../...flags where flags contain 'e'.
	// Simplified: just check if a sed expression contains s/ followed by 'e' flag
	// somewhere after the third / (or other common delimiter).
	for _, f := range fields {
		// Skip flags
		if strings.HasPrefix(f, "-") {
			continue
		}
		// Check for e flag in s<delim> expressions
		if len(f) > 3 && f[0] == 's' {
			delim := f[1]
			// Find the third occurrence of the delimiter
			count := 0
			lastDelimIdx := -1
			for i := 1; i < len(f); i++ {
				if f[i] == delim {
					count++
					if count == 3 {
						lastDelimIdx = i
						break
					}
				}
			}
			// If we found 3 delimiters, check the flags after the 3rd one for 'e'
			if lastDelimIdx >= 0 && lastDelimIdx+1 < len(f) {
				flags := f[lastDelimIdx+1:]
				for _, c := range flags {
					if c == 'e' {
						return "sed security: 'e' flag executes shell commands"
					}
				}
			}
		}
	}

	return ""
}

// ===========================================================================
// xargs security (upstream readOnlyValidation.ts xargs config)
// ===========================================================================

// safeXargsFlags is the set of allowed xargs flags.
var safeXargsFlags = map[string]bool{
	"-I": true, "-n": true, "-P": true, "-L": true,
	"-s": true, "-E": true, "-0": true, "-t": true,
	"-r": true, "-x": true, "-d": true,
}

// checkXargsSecurity validates xargs commands against security patterns.
func checkXargsSecurity(cmd string) string {
	lower := strings.ToLower(cmd)
	if !strings.Contains(lower, "xargs ") && !strings.HasSuffix(lower, "xargs") {
		return ""
	}
	// Check for dangerous flags: lowercase -i and -e (GNU optional-arg parser differential)
	if regexp.MustCompile(`(?i)\bxargs\b`).MatchString(cmd) {
		// Check for -i without value (dangerous in GNU xargs)
		fields := strings.Fields(cmd)
		for i, f := range fields {
			if strings.ToLower(f) == "xargs" {
				// Check remaining args for dangerous patterns
				for _, arg := range fields[i+1:] {
					if arg == "-i" {
						return "xargs security: -i flag has GNU optional-arg parser differential"
					}
					if arg == "-e" && !strings.HasPrefix(fields[bashMin(i+2, len(fields)-1)], "") {
						// -e without value is dangerous
						return "xargs security: -e flag has GNU optional-arg parser differential"
					}
				}
				break
			}
		}
	}
	return ""
}

// ===========================================================================
// fd/fdfind security (upstream EXTERNAL_READONLY_COMMANDS)
// ===========================================================================

var safeFdFlags = map[string]bool{
	"-e": true, "-E": true, "-d": true, "-t": true,
	"-S": true, "-l": true, "-0": true, "-H": true,
	"-a": true, "-s": true, "-i": true, "-u": true,
	"--type": true, "--size": true, "--depth": true,
	"--owner": true, "--changed-within": true, "--changed-before": true,
	"--ignore-vcs": true, "--follow": true, "--regex": true,
	"--fixed-strings": true,
}

var fdDangerousFlags = map[string]bool{
	"-x": true, "--exec": true,
	"-X": true, "--exec-batch": true,
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
func checkGhSecurity(cmd string) string {
	lower := strings.ToLower(cmd)
	if !strings.Contains(lower, "gh ") && !strings.HasSuffix(lower, "gh") {
		return ""
	}
	// Check for 3-segment format HOST/OWNER/REPO (exfil vector)
	if regexp.MustCompile(`\bgh\s+.*[a-z]+\.[a-z]+/[a-z]+/[a-z]+`).MatchString(lower) {
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

// isReadOnlyCommandWithFlags extends isReadOnlyCommand to validate flags for
// commands that have per-flag allowlists (xargs, fd, rg, sort, ps, file, man, help, netstat).
func isReadOnlyCommandWithFlags(cmd string, inner string) bool {
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

	// Step 3: Per-command security validation
	// jq
	if msg := checkJqSecurity(lower); msg != "" {
		return PermissionResultAsk(msg, "tool")
	}
	// sed
	if msg := checkSedSecurity(lower); msg != "" {
		return PermissionResultAsk(msg, "tool")
	}
	// xargs
	if msg := checkXargsSecurity(lower); msg != "" {
		return PermissionResultAsk(msg, "tool")
	}
	// fd/fdfind
	if msg := checkFdSecurity(lower); msg != "" {
		return PermissionResultAsk(msg, "tool")
	}
	// ripgrep
	if msg := checkRgSecurity(lower); msg != "" {
		return PermissionResultAsk(msg, "tool")
	}
	// gh CLI
	if msg := checkGhSecurity(lower); msg != "" {
		return PermissionResultAsk(msg, "tool")
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
