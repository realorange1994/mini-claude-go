package tools

import (
	"regexp"
	"strings"
)

// psSecurityPattern represents a detected security concern in a PowerShell command.
type psSecurityPattern struct {
	name     string
	re       *regexp.Regexp
	severity string // "deny" or "ask"
}

// psSecurityPatterns is the ordered list of security checks.
// Matches upstream powershellSecurity.ts checks (24 sequential checks reduced
// to the most impactful regex-based patterns for Go).
var psSecurityPatterns = []psSecurityPattern{
	// --- DENY patterns (hard blocks) ---
	{
		name: "Invoke-Expression / iex",
		re:   regexp.MustCompile(`(?i)\b(?:invoke-expression|iex)\b`),
		severity: "deny",
	},
	{
		name: "EncodedCommand",
		re:   regexp.MustCompile(`(?i)-encodedcommand\s|-enc\s`),
		severity: "deny",
	},
	{
		name: "Download cradle (download|pipe|execute)",
		re:   regexp.MustCompile(`(?i)(?:invoke-webrequest|invoke-restmethod|iwr|irm)\b.*\|.*\b(?:invoke-expression|iex)\b`),
		severity: "deny",
	},
	{
		name: "Script block execution",
		re:   regexp.MustCompile(`&\s*\{`),
		severity: "deny",
	},
	{
		name: "COM object creation",
		re:   regexp.MustCompile(`(?i)new-object\s+-comobject\b|createobject\(`),
		severity: "deny",
	},
	{
		name: "Bypass execution policy",
		re:   regexp.MustCompile(`(?i)-executionpolicy\s+bypass\b|-ep\s+bypass\b`),
		severity: "deny",
	},
	{
		name: "Base64 decode execution",
		re:   regexp.MustCompile(`(?i)\[convert\]::frombase64string`),
		severity: "deny",
	},
	// --- ASK patterns (user approval required) ---
	{
		name: "Download with file output",
		re:   regexp.MustCompile(`(?i)(?:invoke-webrequest|iwr|irm)\b.*-outfile\b`),
		severity: "ask",
	},
	{
		name: "Reflection / type invocation",
		re:   regexp.MustCompile(`\[.*\]::`),
		severity: "ask",
	},
	{
		name: "Add-Type (compile and load .NET code)",
		re:   regexp.MustCompile(`(?i)\badd-type\b`),
		severity: "ask",
	},
	{
		name: "Hidden window",
		re:   regexp.MustCompile(`(?i)-windowstyle\s+hidden\b|-w\s+hidden\b`),
		severity: "ask",
	},
	{
		name: "Subexpression",
		re:   regexp.MustCompile(`\$\(`),
		severity: "ask",
	},
	{
		name: "Environment variable access",
		re:   regexp.MustCompile(`\$env:`),
		severity: "ask",
	},
	{
		name: "Home directory variable",
		re:   regexp.MustCompile(`\$home\\|\$home/`),
		severity: "ask",
	},
}

// psReadOnlyAllowlist maps lowercase cmdlet names to their allowed flags.
// An empty slice means all flags are allowed (pure read-only cmdlets).
// Sourced from upstream readOnlyValidation.ts cmdlet allowlist.
var psReadOnlyAllowlist = map[string][]string{
	"get-childitem":        {"-Path", "-LiteralPath", "-Filter", "-Recurse", "-Depth", "-Name", "-Force", "-Directory", "-File", "-Hidden", "-ReadOnly", "-System", "-Attributes", "-Include", "-Exclude"},
	"get-content":          {"-Path", "-LiteralPath", "-TotalCount", "-Head", "-Tail", "-Raw", "-Encoding", "-Delimiter", "-ReadCount"},
	"get-item":             {"-Path", "-LiteralPath", "-Force", "-Stream"},
	"get-itemproperty":     {"-Path", "-LiteralPath", "-Name"},
	"test-path":            {"-Path", "-LiteralPath", "-PathType", "-Filter", "-Include", "-Exclude", "-IsValid", "-NewerThan", "-OlderThan"},
	"get-filehash":         {"-Path", "-LiteralPath", "-Algorithm", "-InputStream"},
	"get-acl":              {"-Path", "-LiteralPath", "-Audit", "-Filter", "-Include", "-Exclude"},
	"select-string":        {"-Path", "-LiteralPath", "-Pattern", "-InputObject", "-SimpleMatch", "-CaseSensitive", "-Quiet", "-List", "-NotMatch", "-AllMatches", "-Encoding", "-Context", "-Raw", "-NoEmphasis"},
	"get-process":          {"-Name", "-Id", "-Module", "-FileVersionInfo", "-IncludeUserName"},
	"get-service":          {"-Name", "-DisplayName", "-DependentServices", "-RequiredServices", "-Include", "-Exclude"},
	"get-location":         {"-PSProvider", "-PSDrive", "-Stack", "-StackName"},
	"get-date":             {"-Date", "-Format", "-UFormat", "-DisplayHint", "-AsUTC"},
	"get-host":             {}, // allowAllFlags
	"get-computerinfo":     {}, // allowAllFlags
	"get-psdrive":          {"-Name", "-PSProvider", "-Scope"},
	"get-psprovider":       {"-PSProvider"},
	"get-volume":           {},
	"get-disk":             {},
	"get-hotfix":           {"-Id", "-Description"},
	"get-itempropertyvalue": {"-Path", "-LiteralPath", "-Name"},
	"format-list":          {"-Property", "-GroupBy"},
	"format-table":         {"-Property", "-AutoSize", "-GroupBy", "-HideTableHeaders"},
	"format-wide":          {"-Property", "-AutoSize", "-GroupBy"},
	"format-hex":           {"-Path", "-LiteralPath", "-InputObject", "-Encoding", "-Count", "-Offset"},
	"out-null":             {},
	"out-default":          {},
	"out-string":           {"-Width", "-Stream"},
	"measure-object":       {"-Property", "-Sum", "-Average", "-Maximum", "-Minimum", "-Line", "-Word", "-Character"},
	"sort-object":          {"-Property", "-Descending", "-Unique", "-Top", "-Stable"},
	"select-object":        {"-Property", "-First", "-Last", "-Skip", "-Unique", "-ExpandProperty"},
	"where-object":         {"-Property", "-Value", "-Match"},
	"foreach-object":       {"-Process", "-Begin", "-End"},
	"group-object":         {"-Property", "-NoElement", "-AsHashTable", "-AsString"},
	"convertto-json":       {"-InputObject", "-Depth", "-Compress", "-EnumsAsStrings", "-AsArray"},
	"convertfrom-json":     {"-InputObject", "-Depth", "-AsHashtable", "-NoEnumerate"},
	"convertto-csv":        {"-InputObject", "-Delimiter", "-NoTypeInformation", "-NoHeader", "-UseQuotes"},
	"convertfrom-csv":      {"-InputObject", "-Delimiter", "-Header", "-UseCulture"},
	"convertto-html":       {"-InputObject", "-Property", "-Head", "-Title", "-Body", "-Pre", "-Post", "-As", "-Fragment"},
	"convertto-xml":        {"-InputObject", "-Depth", "-As", "-NoTypeInformation"},
	"compare-object":       {"-ReferenceObject", "-DifferenceObject", "-Property", "-IncludeEqual", "-ExcludeDifferent", "-PassThru"},
	"get-unique":           {"-InputObject", "-AsString", "-CaseInsensitive", "-OnType"},
	"get-member":           {"-InputObject", "-MemberType", "-Name", "-Static", "-View", "-Force"},
	"write-output":         {"-InputObject", "-NoEnumerate"},
	"write-host":           {"-ForegroundColor", "-BackgroundColor", "-NoNewline", "-Separator", "-Object"},
	"write-error":          {"-Message", "-Exception", "-ErrorId", "-Category", "-TargetObject"},
	"write-warning":        {"-Message"},
	"set-location":         {"-Path", "-LiteralPath", "-PassThru", "-StackName"},
	"push-location":        {"-Path", "-LiteralPath", "-PassThru", "-StackName"},
	"pop-location":         {"-PassThru", "-StackName"},
	"join-path":            {"-Path", "-ChildPath", "-AdditionalChildPath", "-Resolve", "-Credential"},
	"split-path":           {"-Path", "-LiteralPath", "-Qualifier", "-NoQualifier", "-Parent", "-Leaf", "-LeafBase", "-Extension", "-IsAbsolute"},
	"resolve-path":         {"-Path", "-LiteralPath", "-Relative"},
	"convert-path":         {"-Path", "-LiteralPath"},
	"get-random":           {"-InputObject", "-Minimum", "-Maximum", "-Count", "-SetSeed", "-Shuffle"},
	"start-sleep":          {"-Seconds", "-Milliseconds"},
	"get-module":           {"-Name", "-ListAvailable", "-FullyQualifiedName"},
	"get-help":             {"-Name", "-Examples", "-Full", "-Detailed", "-Online"},
	"hostname":             {},
}

// psCmdletAliases maps common PowerShell aliases to their canonical cmdlet names.
var psCmdletAliases = map[string]string{
	"ls":   "get-childitem",
	"dir":  "get-childitem",
	"gci":  "get-childitem",
	"cat":  "get-content",
	"gc":   "get-content",
	"type": "get-content",
	"gi":   "get-item",
	"gp":   "get-itemproperty",
	"sl":   "set-location",
	"cd":   "set-location",
	"sls":  "select-string",
	"select": "select-object",
	"where":  "where-object",
	"foreach": "foreach-object",
	"sort":   "sort-object",
	"measure": "measure-object",
	"group":  "group-object",
	"echo":   "write-output",
	"write":  "write-output",
	"cls":    "clear-host",
	"clear":  "clear-host",
}

// IsPowerShellCommand detects if a command string appears to be PowerShell syntax.
// This is used when we can't determine the shell ahead of time (e.g. compound
// commands). On Windows without Git Bash, PowerShell is the default shell.
func IsPowerShellCommand(cmd string) bool {
	lower := strings.ToLower(cmd)

	// Check for Verb-Noun cmdlet pattern (PowerShell's naming convention).
	// Matches patterns like "Get-ChildItem", "Set-Content", "Remove-Item" etc.
	// Must appear as a standalone word (not embedded in another word).
	fields := strings.Fields(lower)
	for _, f := range fields {
		// Strip leading path prefixes like .\ or C:\...
		cleaned := f
		if strings.HasPrefix(cleaned, `.\`) {
			cleaned = cleaned[2:]
		}
		// Check for Verb-Noun pattern (hyphen between uppercase-starting words)
		if isPsCmdletPattern(cleaned) != "" {
			return true
		}
		// Check aliases (case-insensitive)
		if _, ok := psCmdletAliases[cleaned]; ok {
			// But skip ambiguous aliases that also work in bash: ls, cat, echo, cd, dir
			// These are only treated as PS when other PS markers are present
			switch cleaned {
			case "ls", "cat", "echo", "cd", "dir", "type", "clear":
				continue // ambiguous — don't classify as PS on alias alone
			default:
				return true
			}
		}
	}

	// Check for PowerShell-specific syntax that bash doesn't support
	psSyntaxMarkers := []string{
		"invoke-", "iex ", "iwr ", "irm ",
		"$(", "${", "@{", "@(",
		"-comobject",
		"-encodedcommand", "-enc ",
		"-executionpolicy", "-ep ",
		"-windowstyle", "-verb ",
		"[convert]::", "[system.", "[microsoft.",
		"$env:", "$home\\", "$home/",
		"new-object", "add-type",
	}

	for _, pat := range psSyntaxMarkers {
		if strings.Contains(lower, pat) {
			return true
		}
	}

	return false
}

// isPsCmdletPattern checks if a word matches PowerShell's Verb-Noun naming convention.
// Valid PowerShell verbs include: Get, Set, New, Remove, Add, Invoke, Start, Stop,
// Clear, Copy, Move, Rename, Select, Format, Out, Write, Read, Update, Register,
// Unregister, Enable, Disable, Test, Measure, Sort, Group, Compare, Convert,
// Join, Split, Resolve, Push, Pop, ForEach, Where, etc.
func isPsCmdletPattern(word string) string {
	if len(word) < 4 {
		return ""
	}
	idx := strings.Index(word, "-")
	if idx < 1 || idx == len(word)-1 {
		return ""
	}
	verb := word[:idx]
	// Check if the verb is a known PowerShell verb
	psVerbs := map[string]bool{
		"get": true, "set": true, "new": true, "remove": true, "add": true,
		"invoke": true, "start": true, "stop": true, "clear": true,
		"copy": true, "move": true, "rename": true, "select": true,
		"format": true, "out": true, "write": true, "read": true,
		"update": true, "register": true, "unregister": true,
		"enable": true, "disable": true, "test": true, "measure": true,
		"sort": true, "group": true, "compare": true, "convert": true,
		"convertto": true, "convertfrom": true, "join": true, "split": true,
		"resolve": true, "push": true, "pop": true, "foreach": true,
		"where": true, "debug": true, "enter": true,
		"exit": true, "return": true, "throw": true,
		"use": true, "publish": true, "unblock": true,
		"install": true, "save": true, "load": true, "restore": true,
		"backup": true, "recover": true, "merge": true, "import": true,
		"export": true, "receive": true, "send": true, "connect": true,
		"disconnect": true, "grant": true, "revoke": true, "lock": true,
		"unlock": true, "protect": true, "unprotect": true, "audit": true,
		"assert": true, "confirm": true, "deny": true, "approve": true,
		"complete": true, "configure": true, "initialize": true,
		"limit": true, "suspend": true, "resume": true, "restart": true,
		"reset": true, "search": true, "trace": true, "watch": true,
		"hide": true, "show": true, "open": true, "close": true,
		"block": true, "resize": true, "optimize": true,
		"pack": true, "unpack": true, "unpublish": true,
		"flush": true, "peek": true, "skip": true, "step": true,
		"switch": true, "wait": true, "trap": true,
	}
	if psVerbs[verb] {
		return word
	}
	return ""
}

// checkPsSecurityPatterns scans the command for known dangerous PowerShell patterns.
// Returns a list of messages for each pattern found. Empty = no patterns matched.
// Patterns are classified as "deny" (hard block) or "ask" (user approval needed).
func checkPsSecurityPatterns(cmd string) (denyMsgs, askMsgs []string) {
	for _, pat := range psSecurityPatterns {
		if pat.re.MatchString(cmd) {
			msg := "PowerShell security: " + pat.name
			if pat.severity == "deny" {
				denyMsgs = append(denyMsgs, msg)
			} else {
				askMsgs = append(askMsgs, msg)
			}
		}
	}
	return denyMsgs, askMsgs
}

// isPsReadOnlyCommand checks if a command uses only read-only cmdlets with
// allowed flags. It extracts the first cmdlet from the command (before pipes
// or semicolons), resolves aliases, and validates all flags against the allowlist.
func isPsReadOnlyCommand(cmd string) bool {
	// Extract first segment (before | or ;)
	first := cmd
	if idx := strings.IndexAny(first, "|;"); idx >= 0 {
		first = first[:idx]
	}
	fields := strings.Fields(first)
	if len(fields) == 0 {
		return false
	}

	// Get cmdlet name, resolve alias
	cmdlet := strings.ToLower(fields[0])
	if canonical, ok := psCmdletAliases[cmdlet]; ok {
		cmdlet = canonical
	}

	// Check if in allowlist
	safeFlags, ok := psReadOnlyAllowlist[cmdlet]
	if !ok {
		return false
	}

	// If allowlist has no flags (allowAllFlags), any flags are fine
	if len(safeFlags) == 0 {
		return true
	}

	// Validate that every argument starting a with '-' is in the safe list
	for _, arg := range fields[1:] {
		if !strings.HasPrefix(arg, "-") {
			continue // positional args are allowed for read-only cmdlets
		}
		// Normalize: -PropertyName -> check exact match and prefix match
		found := false
		argLower := strings.ToLower(arg)
		for _, sf := range safeFlags {
			if strings.ToLower(sf) == argLower || strings.HasPrefix(argLower, strings.ToLower(sf)) {
				found = true
				break
			}
		}
		if !found {
			return false // unknown flag = not purely read-only
		}
	}

	return true
}

// classifyPsVerb extracts the verb from a Verb-Noun pattern and classifies it.
// Returns: "readonly", "write", "destructive", "execution", or "unknown".
func classifyPsVerb(cmdlet string) string {
	// Resolve alias first
	if canonical, ok := psCmdletAliases[cmdlet]; ok {
		cmdlet = canonical
	}

	idx := strings.Index(cmdlet, "-")
	if idx < 1 {
		return "unknown"
	}
	verb := strings.ToLower(cmdlet[:idx])

	switch verb {
	case "get", "select", "format", "out", "measure", "sort", "compare",
		"join", "split", "resolve", "test", "convert", "group", "where",
		"foreach", "hostname", "convertto", "convertfrom":
		return "readonly"
	case "set", "update", "register", "enable", "disable", "rename":
		return "write"
	case "remove", "stop", "kill", "clear", "del", "erase":
		return "destructive"
	case "invoke", "start", "new", "unblock", "publish", "install":
		return "execution"
	default:
		return "unknown"
	}
}

// psReadOnlyCmdletNames returns the set of known read-only cmdlet names
// (including aliases) for quick lookup.
var psReadOnlyCmdletNames = func() map[string]bool {
	m := make(map[string]bool, len(psReadOnlyAllowlist)+len(psCmdletAliases))
	for k := range psReadOnlyAllowlist {
		m[k] = true
	}
	for _, v := range psCmdletAliases {
		m[v] = true
	}
	for k := range psCmdletAliases {
		m[k] = true
	}
	return m
}()

// CheckPowerShellPermission evaluates a PowerShell command against security patterns
// and the read-only allowlist. Returns:
//   - PermissionResultDeny for hard-blocked patterns (Invoke-Expression, encoded commands, etc.)
//   - PermissionResultAsk for suspicious but not outright-blocked patterns
//   - PermissionResultAllow for verified read-only cmdlets
//   - PermissionResultPassthrough if the command doesn't appear to be PowerShell
func CheckPowerShellPermission(cmd string) PermissionResult {
	lower := strings.ToLower(strings.TrimSpace(cmd))

	// Only apply to commands that look like PowerShell
	if !IsPowerShellCommand(lower) {
		return PermissionResultPassthrough()
	}

	// Step 1: Check security patterns — deny
	denyMsgs, askMsgs := checkPsSecurityPatterns(lower)
	if len(denyMsgs) > 0 {
		return PermissionResultDeny(strings.Join(denyMsgs, "; "))
	}

	// Step 2: Check security patterns — ask
	if len(askMsgs) > 0 {
		return PermissionResultAsk(strings.Join(askMsgs, "; "), "tool")
	}

	// Step 3: Check read-only allowlist
	if isPsReadOnlyCommand(lower) {
		return PermissionResultAllow()
	}

	// Step 4: Classify the first cmdlet's verb for targeted messages
	first := lower
	if idx := strings.IndexAny(first, "|;"); idx >= 0 {
		first = first[:idx]
	}
	fields := strings.Fields(first)
	if len(fields) > 0 {
		cmdlet := fields[0]
		if canonical, ok := psCmdletAliases[cmdlet]; ok {
			cmdlet = canonical
		}
		verbClass := classifyPsVerb(cmdlet)

		switch verbClass {
		case "readonly":
			// Known read-only verb but not in allowlist (new cmdlet or unusual flags)
			// Fall through to ask
			return PermissionResultAsk("PowerShell cmdlet '"+cmdlet+"' requires verification", "tool")
		case "destructive":
			return PermissionResultAsk("PowerShell destructive cmdlet '"+cmdlet+"' requires approval", "tool")
		case "execution":
			return PermissionResultAsk("PowerShell execution cmdlet '"+cmdlet+"' requires approval", "tool")
		case "write":
			return PermissionResultAsk("PowerShell write cmdlet '"+cmdlet+"' requires approval", "tool")
		}
	}

	// Step 5: Unknown PowerShell command — ask
	return PermissionResultAsk("Unrecognized PowerShell command requires approval", "tool")
}
