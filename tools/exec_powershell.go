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
// Matches upstream powershellSecurity.ts checks (26 AST-based checks reduced
// to the most impactful regex-based patterns for Go).
var psSecurityPatterns = []psSecurityPattern{
	// --- DENY patterns (hard blocks) ---

	// Upstream #1: checkInvokeExpression — Invoke-Expression / iex
	{
		name:     "Invoke-Expression / iex",
		re:       regexp.MustCompile(`(?i)\b(?:invoke-expression|iex)\b`),
		severity: "deny",
	},
	// Upstream #3: checkEncodedCommand — -EncodedCommand/-Enc
	{
		name:     "EncodedCommand",
		re:       regexp.MustCompile(`(?i)(?:-encodedcommand\s|-enc\s|/encodedcommand\s)`),
		severity: "deny",
	},
	// Upstream #5: checkDownloadCradles — piped download+execute
	{
		name:     "Download cradle (download|pipe|execute)",
		re:       regexp.MustCompile(`(?i)(?:invoke-webrequest|invoke-restmethod|iwr|irm)\b.*\|.*\b(?:invoke-expression|iex)\b`),
		severity: "deny",
	},
	// Upstream #5 (cross-statement): split download cradle ($r = IWR ...; IEX $r.Content)
	{
		name:     "Download cradle (cross-statement)",
		re:       regexp.MustCompile(`(?i)\$\w+\s*=\s*(?:invoke-webrequest|invoke-restmethod|iwr|irm)\b.*;\s*\b(?:invoke-expression|iex)\b`),
		severity: "deny",
	},
	// Upstream #6: checkDownloadUtilities — certutil, bitsadmin, Start-BitsTransfer
	{
		name:     "Download utility (certutil/bitsadmin/Start-BitsTransfer)",
		re:       regexp.MustCompile(`(?i)(?:certutil\s+-urlcache|bitsadmin\s+/transfer|start-bitstransfer)\b`),
		severity: "deny",
	},
	// Upstream #4: checkPwshCommandOrFile — PowerShell re-invocation
	{
		name:     "PowerShell re-invocation (pwsh/powershell.exe)",
		re:       regexp.MustCompile(`(?i)\b(?:pwsh|powershell)(?:\.exe)?\b(?:\s+-|.*-command|.*-file|.*-encoded)`),
		severity: "deny",
	},
	// Script block execution (& { ... })
	{
		name:     "Script block execution",
		re:       regexp.MustCompile(`&\s*\{`),
		severity: "deny",
	},
	// Upstream #8: checkComObject — COM object creation
	{
		name:     "COM object creation",
		re:       regexp.MustCompile(`(?i)new-object\s+-comobject\b|createobject\(`),
		severity: "deny",
	},
	// Bypass execution policy
	{
		name:     "Bypass execution policy",
		re:       regexp.MustCompile(`(?i)-executionpolicy\s+bypass\b|-ep\s+bypass\b`),
		severity: "deny",
	},
	// Base64 decode execution
	{
		name:     "Base64 decode execution",
		re:       regexp.MustCompile(`(?i)\[convert\]::frombase64string`),
		severity: "deny",
	},
	// Upstream #19: checkInvokeItem — Invoke-Item / ii
	{
		name:     "Invoke-Item / ii",
		re:       regexp.MustCompile(`(?i)\b(?:invoke-item|ii)\b`),
		severity: "deny",
	},
	// Upstream #20: checkScheduledTask — scheduled task persistence
	{
		name:     "Scheduled task creation",
		re:       regexp.MustCompile(`(?i)(?:register-scheduledtask|schtasks\s+/create)\b`),
		severity: "deny",
	},
	// Upstream #24: checkWmiProcessSpawn — WMI/CIM process spawn
	{
		name:     "WMI/CIM process spawn",
		re:       regexp.MustCompile(`(?i)\b(?:invoke-wmimethod|invoke-cimmethod)\b`),
		severity: "deny",
	},

	// --- ASK patterns (user approval required) ---

	// Upstream #5 (variant): download with file output
	{
		name:     "Download with file output",
		re:       regexp.MustCompile(`(?i)(?:invoke-webrequest|iwr|irm)\b.*-outfile\b`),
		severity: "ask",
	},
	// Upstream #9: checkDangerousFilePathExecution — Invoke-Command -FilePath, Start-Job -FilePath
	{
		name:     "Dangerous file path execution",
		re:       regexp.MustCompile(`(?i)\b(?:invoke-command|start-job)\b.*-filepath\b`),
		severity: "ask",
	},
	// Upstream #11: checkStartProcess — Start-Process -Verb RunAs
	{
		name:     "Start-Process RunAs",
		re:       regexp.MustCompile(`(?i)\bstart-process\b.*-verb\s+(?:runas|runas:)`),
		severity: "ask",
	},
	// Upstream #11 (variant): Start-Process targeting PowerShell
	{
		name:     "Start-Process targeting PowerShell",
		re:       regexp.MustCompile(`(?i)\bstart-process\b.*(?:pwsh|powershell)`),
		severity: "ask",
	},
	// Upstream #12: checkScriptBlockInjection — dangerous script block cmdlets
	{
		name:     "Script block injection",
		re:       regexp.MustCompile(`(?i)\b(?:invoke-command|start-job|register-wmievent|register-cimindicationquery)\b.*-scriptblock\b`),
		severity: "ask",
	},
	// Upstream #17: Reflection / type invocation
	{
		name:     "Reflection / type invocation",
		re:       regexp.MustCompile(`\[.*\]::`),
		severity: "ask",
	},
	// Upstream #7: Add-Type (compile and load .NET code)
	{
		name:     "Add-Type (compile and load .NET code)",
		re:       regexp.MustCompile(`(?i)\badd-type\b`),
		severity: "ask",
	},
	// Upstream #10: checkForEachMemberName — ForEach-Object -MemberName
	{
		name:     "ForEach-Object -MemberName (method invocation)",
		re:       regexp.MustCompile(`(?i)\b(?:foreach-object|%)\b.*-membername\b`),
		severity: "ask",
	},
	// Upstream #21: checkEnvVarManipulation — env var write operations
	{
		name:     "Environment variable manipulation",
		re:       regexp.MustCompile(`(?i)\b(?:set-item|remove-item|clear-item)\s+(?:env:|environment::)`),
		severity: "ask",
	},
	// Upstream #22: checkModuleLoading — Import-Module, Install-Module, Save-Module
	{
		name:     "Module loading/installation",
		re:       regexp.MustCompile(`(?i)\b(?:import-module|ipmo|install-module|save-module)\b`),
		severity: "ask",
	},
	// Upstream #23: checkRuntimeStateManipulation — Set-Alias, Set-Variable, New-Alias, New-Variable
	{
		name:     "Runtime state manipulation",
		re:       regexp.MustCompile(`(?i)\b(?:set-alias|new-alias|set-variable|sv|new-variable|nv)\b`),
		severity: "ask",
	},
	// Hidden window
	{
		name:     "Hidden window",
		re:       regexp.MustCompile(`(?i)-windowstyle\s+hidden\b|-w\s+hidden\b`),
		severity: "ask",
	},
	// Upstream #13: Subexpression
	{
		name:     "Subexpression",
		re:       regexp.MustCompile(`\$\(`),
		severity: "ask",
	},
	// Upstream #14: Expandable strings / environment variable access
	{
		name:     "Environment variable access",
		re:       regexp.MustCompile(`\$env:`),
		severity: "ask",
	},
	// Home directory variable
	{
		name:     "Home directory variable",
		re:       regexp.MustCompile(`\$home\\|\$home/`),
		severity: "ask",
	},
	// Upstream #15: checkSplatting — splatting (@var)
	{
		name:     "Splatting (@variable)",
		re:       regexp.MustCompile(`@\w+`),
		severity: "ask",
	},
	// Upstream #16: checkStopParsing — stop-parsing token (--)
	{
		name:     "Stop-parsing token (--%)",
		re:       regexp.MustCompile(`--%`),
		severity: "ask",
	},
	// Upstream pathValidation: UNC path blocking
	{
		name:     "UNC path access",
		re:       regexp.MustCompile(`(?:\\\\|//)\S+`),
		severity: "ask",
	},
	// Upstream readOnlyValidation: Provider path detection
	{
		name:     "Non-filesystem provider path",
		re:       regexp.MustCompile(`(?i)\b(?:env:|HKLM:|HKCU:|function:|alias:|variable:|cert:|wsman:)`),
		severity: "ask",
	},
	// Upstream #25: using statements
	{
		name:     "Using statement",
		re:       regexp.MustCompile(`(?i)\busing\s+(?:namespace|module|assembly)\b`),
		severity: "ask",
	},
	// Upstream #26: #Requires directive
	{
		name:     "#Requires directive",
		re:       regexp.MustCompile(`(?i)#requires\s`),
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
	// Additional cmdlets from upstream readOnlyValidation.ts CMDLET_ALLOWLIST
	"get-netipconfiguration": {},
	"get-netadapter":         {},
	"get-netroute":           {},
	"test-connection":        {"-ComputerName", "-Count", "-Quiet", "-Source", "-Destination"},
	"get-eventlog":           {"-LogName", "-Newest", "-EntryType", "-Source", "-Message", "-InstanceId", "-After", "-Before"},
	"get-wmiobject":          {"-Class", "-Query", "-Namespace", "-ComputerName", "-Filter", "-Property"},
	"get-ciminstance":        {"-ClassName", "-Query", "-Namespace", "-ComputerName", "-Filter", "-Property"},
	"get-culture":            {},
	"get-uiculture":          {},
	"get-timezone":           {},
	"get-winssystemlocale":   {},
	"get-pssession":          {"-Name", "-Id", "-InstanceId", "-ComputerName", "-ConfigurationName"},
	"get-command":            {"-Name", "-CommandType", "-Module", "-Syntax", "-Verb", "-Noun"},
	"get-history":            {"-Id", "-Count"},
	"get-alias":              {"-Name", "-Definition", "-Exclude"},
	"get-variable":           {"-Name", "-ValueOnly", "-Scope", "-Include", "-Exclude"},
	"get-cred":               {},
}

// psCmdletAliases maps common PowerShell aliases to their canonical cmdlet names.
// Sourced from upstream powershellPermissions.ts COMMON_ALIASES.
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
	// Additional aliases from upstream COMMON_ALIASES
	"ii":     "invoke-item",
	"saps":   "start-process",
	"start":  "start-process",
	"ndr":    "new-psdrive",
	"mount":  "new-psdrive",
	"sal":    "set-alias",
	"sv":     "set-variable",
	"nv":     "new-variable",
	"ipmo":   "import-module",
	"iwmi":   "invoke-wmimethod",
	"icm":    "invoke-command",
	"sajb":   "start-job",
	"rbp":    "remove-psbreakpoint",
	// Destructive aliases — map to Remove-Item so security patterns catch them.
	// Without these, `rm ./x` bypasses the Remove-Item deny patterns.
	"del":   "remove-item",
	"rm":    "remove-item",
	"ri":    "remove-item",
	"erase": "remove-item",
	"rd":    "remove-item",
	"rmdir": "remove-item",
	"rp":    "remove-itemproperty",
	"rni":   "rename-item",
	"rmp":   "remove-itemproperty",
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

// ===========================================================================
// Collect-then-reduce decision model (upstream powershellPermissions.ts)
// ===========================================================================
// Upstream uses a decision array and reduces with deny > ask > allow precedence.
// This structurally prevents ask-before-deny bugs where an early ask masks a
// later deny.

// psDecision represents a single permission decision with its behavior and reason.
type psDecision struct {
	behavior string // "deny", "ask", "allow"
	message  string
}

// psDecisionCollector collects security decisions and reduces them with
// deny > ask > allow precedence (upstream collect-then-reduce model).
type psDecisionCollector struct {
	denials []psDecision
	asks    []psDecision
	allows  []psDecision
}

func (c *psDecisionCollector) deny(msg string) {
	c.denials = append(c.denials, psDecision{behavior: "deny", message: msg})
}
func (c *psDecisionCollector) ask(msg string) {
	c.asks = append(c.asks, psDecision{behavior: "ask", message: msg})
}
func (c *psDecisionCollector) allow(msg string) {
	c.allows = append(c.allows, psDecision{behavior: "allow", message: msg})
}

// reduce returns the first decision in deny > ask > allow precedence.
// Returns nil if no decision was made (caller should use passthrough).
func (c *psDecisionCollector) reduce() *PermissionResult {
	if len(c.denials) > 0 {
		msgs := make([]string, 0, len(c.denials))
		for _, d := range c.denials {
			msgs = append(msgs, d.message)
		}
		r := PermissionResultDeny(strings.Join(msgs, "; "))
		return &r
	}
	if len(c.asks) > 0 {
		msgs := make([]string, 0, len(c.asks))
		for _, d := range c.asks {
			msgs = append(msgs, d.message)
		}
		r := PermissionResultAsk(strings.Join(msgs, "; "), "tool")
		return &r
	}
	if len(c.allows) > 0 {
		r := PermissionResultAllow()
		return &r
	}
	return nil
}

// ===========================================================================
// Fragment-based deny scanning for parse-failed commands (upstream step 8-10)
// ===========================================================================

// psSplitSeparators splits a command on common separators for fragment-based
// fallback when AST parsing fails. Matches upstream separator pattern.
var psSplitSeparators = regexp.MustCompile(`[;|&]+`)

// psNormalizeFragment normalizes a command fragment for deny rule matching.
// Strips assignment prefixes ($x = $y = ...) and invocation prefixes (&, .).
// Matches upstream normalization logic.
var psAssignmentPrefixRe = regexp.MustCompile(`^\$[\w:]+\s*(?:[+\-*/%]|\?\?)?\s*=\s*`)

func psNormalizeFragment(fragment string) string {
	fragment = strings.TrimSpace(fragment)

	// Strip nested assignment prefixes (e.g., "$x = $y = iex" -> "iex")
	for {
		if !psAssignmentPrefixRe.MatchString(fragment) {
			break
		}
		fragment = psAssignmentPrefixRe.ReplaceAllString(fragment, "")
		fragment = strings.TrimSpace(fragment)
	}

	// Strip invocation/dot-source prefixes
	if strings.HasPrefix(fragment, "& ") {
		fragment = strings.TrimSpace(fragment[2:])
	}
	if strings.HasPrefix(fragment, ". ") {
		fragment = strings.TrimSpace(fragment[2:])
	}

	// Strip surrounding quotes from first token
	if len(fragment) > 2 {
		if (fragment[0] == '"' && fragment[len(fragment)-1] == '"') ||
			(fragment[0] == '\'' && fragment[len(fragment)-1] == '\'') {
			fragment = fragment[1 : len(fragment)-1]
		}
	}

	return strings.ToLower(fragment)
}

// scanFragmentForDenial checks if a normalized fragment matches any deny rules.
// Returns a denial message if found, empty string if safe.
func scanFragmentForDenial(fragment string) string {
	// Check if fragment matches invoke-expression/iex
	if strings.Contains(fragment, "invoke-expression") || strings.Contains(fragment, "iex") {
		return "PowerShell security: Invoke-Expression detected in command fragment"
	}
	// Check for encoded command
	if strings.Contains(fragment, "-encodedcommand") || strings.Contains(fragment, "-enc ") {
		return "PowerShell security: EncodedCommand detected"
	}
	// Check for download cradle patterns
	if strings.Contains(fragment, "invoke-webrequest") || strings.Contains(fragment, "iwr") {
		// Cross-reference with other fragments checked in caller
	}
	return ""
}

// scanFragmentsForDenial scans command fragments for denial patterns.
// Used when AST parsing fails and we need to do fragment-based fallback.
func scanFragmentsForDenial(fragments []string) string {
	// First pass: check each fragment for individual deny patterns
	for _, frag := range fragments {
		if msg := scanFragmentForDenial(frag); msg != "" {
			return msg
		}
	}

	// Second pass: check for cross-statement patterns
	// (e.g., one fragment has downloader, another has IEX)
	hasDownloader := false
	for _, frag := range fragments {
		fragLower := strings.ToLower(frag)
		if strings.Contains(fragLower, "invoke-webrequest") ||
			strings.Contains(fragLower, "invoke-restmethod") ||
			strings.Contains(fragLower, "iwr") ||
			strings.Contains(fragLower, "irm") ||
			strings.Contains(fragLower, "start-bitstransfer") {
			hasDownloader = true
			break
		}
	}
	if hasDownloader {
		for _, frag := range fragments {
			fragLower := strings.ToLower(frag)
			if strings.Contains(fragLower, "invoke-expression") ||
				strings.Contains(fragLower, "iex") {
				return "PowerShell security: cross-statement download cradle detected"
			}
		}
	}

	return ""
}

// CheckPowerShellPermission evaluates a PowerShell command against security patterns
// and the read-only allowlist. Uses collect-then-reduce decision model matching
// upstream powershellPermissions.ts. Returns:
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

	// Collect-then-reduce decision model (upstream powershellPermissions.ts)
	// All decisions are collected and reduced with deny > ask > allow precedence.
	// This structurally prevents ask-before-deny bugs.
	collector := &psDecisionCollector{}

	// Step 1: Check security patterns (deny + ask collected separately)
	denyMsgs, askMsgs := checkPsSecurityPatterns(lower)
	for _, msg := range denyMsgs {
		collector.deny(msg)
	}
	for _, msg := range askMsgs {
		collector.ask(msg)
	}

	// Step 2: Check read-only allowlist
	if isPsReadOnlyCommand(lower) {
		collector.allow("PowerShell command is read-only")
	} else {
		// Not in allowlist — classify the verb for targeted messages
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
				// Known read-only verb but not in allowlist
				collector.ask("PowerShell cmdlet '" + cmdlet + "' requires verification")
			case "destructive":
				collector.ask("PowerShell destructive cmdlet '" + cmdlet + "' requires approval")
			case "execution":
				collector.ask("PowerShell execution cmdlet '" + cmdlet + "' requires approval")
			case "write":
				collector.ask("PowerShell write cmdlet '" + cmdlet + "' requires approval")
			default:
				collector.ask("Unrecognized PowerShell command requires approval")
			}
		} else {
			collector.ask("Unrecognized PowerShell command requires approval")
		}
	}

	// Step 3: Fragment-based deny scanning for complex commands (parse-failed fallback)
	// When the command contains multiple sub-commands separated by ;|&, we split
	// and check each fragment individually as defense-in-depth.
	subCmds := splitPsSubCommands(lower)
	if len(subCmds) > 1 {
		fragments := make([]string, 0, len(subCmds))
		for _, sub := range subCmds {
			fragments = append(fragments, psNormalizeFragment(sub))
		}
		if msg := scanFragmentsForDenial(fragments); msg != "" {
			collector.deny(msg)
		}
	}

	// Step 4: Reduce with deny > ask > allow precedence
	if result := collector.reduce(); result != nil {
		return *result
	}

	// Step 5: Fallback — ask for anything not yet decided
	return PermissionResultAsk("PowerShell command requires approval", "tool")
}

// splitPsSubCommands splits a PowerShell command on common separators.
// Used for fragment-based analysis when the command may contain multiple
// sub-commands that need individual security checking.
func splitPsSubCommands(cmd string) []string {
	// Split on semicolons, && and ||
	// Note: we don't split on pipes (|) because piped commands are a single
	// pipeline, not separate sub-commands.
	var result []string
	for _, part := range strings.Split(cmd, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Further split on && and ||
		for _, sub := range psSplitSeparators.Split(part, -1) {
			sub = strings.TrimSpace(sub)
			if sub != "" {
				result = append(result, sub)
			}
		}
	}
	return result
}
