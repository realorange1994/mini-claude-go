package tools

import (
	"strings"
)

// ===========================================================================
// Bash read-only command validation (upstream readOnlyValidation.ts)
// Per-command flag allowlists with FlagArgType validation.
// ===========================================================================

// bashROFlagConfig represents a read-only command's flag validation config.
type bashROFlagConfig struct {
	safeFlags              map[string]string // flag -> FlagArgType ("none"/"number"/"string"/"char"/"{}"/"EOF")
	dangerousFlags         map[string]bool   // explicitly dangerous flags
	additionalDangerous    func(args []string, rawCmd string) bool
	respectsDoubleDash     bool // default true; false for macOS base64 etc.
}

// bashROFlagType constants
const (
	bashRONone   = "none"
	bashRONumber = "number"
	bashROString = "string"
	bashROChar   = "char"
	bashROBrace  = "{}"
	bashROEOF    = "EOF"
)

// bashReadOnlyCommands maps command names to their read-only flag configs.
// This is the main allowlist for commands that run via bash/shell.
var bashReadOnlyCommands = map[string]*bashROFlagConfig{
	// --- xargs ---
	"xargs": {
		safeFlags: map[string]string{
			"-I": bashROBrace, "-n": bashRONumber, "-P": bashRONumber,
			"-L": bashRONumber, "-s": bashRONumber, "-E": bashROEOF,
			"-0": bashRONone, "-t": bashRONone, "-r": bashRONone,
			"-x": bashRONone, "-d": bashROChar,
		},
		dangerousFlags: map[string]bool{
			"-i": true, "-e": true, // GNU optional-arg parser differential
		},
	},

	// --- file ---
	"file": {
		safeFlags: map[string]string{
			"--brief": bashRONone, "-b": bashRONone,
			"--mime": bashRONone, "-i": bashRONone,
			"--mime-type": bashRONone, "--mime-encoding": bashRONone,
			"--apple": bashRONone, "--check-encoding": bashRONone,
			"-c": bashRONone,
			"--exclude": bashROString, "--exclude-quiet": bashROString,
			"--print0": bashRONone, "-0": bashRONone,
			"-f": bashROString, "-F": bashROString,
			"--separator": bashROString,
			"--help": bashRONone, "--version": bashRONone, "-v": bashRONone,
			"--no-dereference": bashRONone, "-h": bashRONone,
			"--dereference": bashRONone, "-L": bashRONone,
			"--magic-file": bashROString, "-m": bashROString,
			"--keep-going": bashRONone, "-k": bashRONone,
			"--list": bashRONone, "-l": bashRONone,
			"--no-buffer": bashRONone, "-n": bashRONone,
			"--preserve-date": bashRONone, "-p": bashRONone,
			"--raw": bashRONone, "-r": bashRONone,
			"-s": bashRONone, "--special-files": bashRONone,
			"--uncompress": bashRONone, "-z": bashRONone,
		},
	},

	// --- sort ---
	"sort": {
		safeFlags: map[string]string{
			"--ignore-leading-blanks": bashRONone, "-b": bashRONone,
			"--dictionary-order": bashRONone, "-d": bashRONone,
			"--ignore-case": bashRONone, "-f": bashRONone,
			"--general-numeric-sort": bashRONone, "-g": bashRONone,
			"--human-numeric-sort": bashRONone, "-h": bashRONone,
			"--ignore-nonprinting": bashRONone, "-i": bashRONone,
			"--month-sort": bashRONone, "-M": bashRONone,
			"--numeric-sort": bashRONone, "-n": bashRONone,
			"--random-sort": bashRONone, "-R": bashRONone,
			"--reverse": bashRONone, "-r": bashRONone,
			"--sort": bashROString,
			"--stable": bashRONone, "-s": bashRONone,
			"--unique": bashRONone, "-u": bashRONone,
			"--version-sort": bashRONone, "-V": bashRONone,
			"--zero-terminated": bashRONone, "-z": bashRONone,
			"--key": bashROString, "-k": bashROString,
			"--field-separator": bashROString, "-t": bashROString,
			"--check": bashRONone, "-c": bashRONone,
			"--check-char-order": bashRONone, "-C": bashRONone,
			"--merge": bashRONone, "-m": bashRONone,
			"--buffer-size": bashROString, "-S": bashROString,
			"--parallel": bashRONumber, "--batch-size": bashRONumber,
			"--help": bashRONone, "--version": bashRONone,
		},
		// -o (output file) intentionally excluded
	},

	// --- man ---
	"man": {
		safeFlags: map[string]string{
			"-a": bashRONone, "--all": bashRONone,
			"-d": bashRONone,
			"-f": bashRONone, "--whatis": bashRONone,
			"-h": bashRONone,
			"-k": bashRONone, "--apropos": bashRONone,
			"-l": bashROString, "-w": bashRONone,
			"-S": bashROString, "-s": bashROString,
		},
		// -P (pager) intentionally excluded — arbitrary command execution
	},

	// --- help ---
	"help": {
		safeFlags: map[string]string{
			"-d": bashRONone,
			"-m": bashRONone,
			"-s": bashRONone,
		},
	},

	// --- netstat ---
	"netstat": {
		safeFlags: map[string]string{
			"-a": bashRONone, "-L": bashRONone,
			"-l": bashRONone, "-n": bashRONone,
			"-t": bashRONone, "-p": bashRONone,
			"-i": bashRONone, "-I": bashROString,
			"-s": bashRONone, "-r": bashRONone,
			"-m": bashRONone, "-v": bashRONone,
		},
	},

	// --- ps ---
	"ps": {
		safeFlags: map[string]string{
			"-e": bashRONone, "-A": bashRONone,
			"-a": bashRONone, "-d": bashRONone,
			"-N": bashRONone, "--deselect": bashRONone,
			"-f": bashRONone, "-F": bashRONone,
			"-l": bashRONone, "-j": bashRONone,
			"-y": bashRONone, "-w": bashRONone,
			"-ww": bashRONone,
			"--width": bashRONumber,
			"-c": bashRONone,
			"-H": bashRONone, "--forest": bashRONone,
			"--headers": bashRONone, "--no-headers": bashRONone,
			"-n": bashROString,
			"--sort": bashROString,
			"-o": bashROString, "--format": bashROString,
			"-L": bashRONone, "-T": bashRONone, "-m": bashRONone,
			"-C": bashROString, "-G": bashROString, "-g": bashROString,
			"-p": bashROString, "--pid": bashROString,
			"-q": bashROString, "--quick-pid": bashROString,
			"-s": bashROString, "--sid": bashROString,
			"-t": bashROString, "--tty": bashROString,
			"-U": bashROString, "-u": bashROString,
			"--user": bashROString,
			"--help": bashRONone, "--info": bashRONone,
			"-V": bashRONone, "--version": bashRONone,
		},
		// Blocks BSD-style 'e' modifier (shows env vars)
		additionalDangerous: func(args []string, rawCmd string) bool {
			for _, a := range args {
				if !strings.HasPrefix(a, "-") {
					// Check if this is BSD-style 'e' modifier
					trimmed := strings.TrimLeft(a, "-")
					if trimmed != "" && len(trimmed) > 0 {
						for _, c := range trimmed {
							if c == 'e' {
								return true
							}
						}
					}
				}
			}
			return false
		},
	},

	// --- base64 ---
	"base64": {
		safeFlags: map[string]string{
			"-d": bashRONone, "-D": bashRONone, "--decode": bashRONone,
			"-b": bashRONumber, "--break": bashRONumber,
			"-w": bashRONumber, "--wrap": bashRONumber,
			"-i": bashROString, "--input": bashROString,
			"--ignore-garbage": bashRONone,
			"-h": bashRONone, "--help": bashRONone, "--version": bashRONone,
		},
		respectsDoubleDash: false, // macOS base64 doesn't respect POSIX --
		// -o/--output (write to file) intentionally excluded
	},

	// --- grep ---
	"grep": {
		safeFlags: map[string]string{
			"-e": bashROString, "--regexp": bashROString,
			"-f": bashROString, "--file": bashROString,
			"-F": bashRONone, "--fixed-strings": bashRONone,
			"-G": bashRONone, "--basic-regexp": bashRONone,
			"-E": bashRONone, "--extended-regexp": bashRONone,
			"-P": bashRONone, "--perl-regexp": bashRONone,
			"-i": bashRONone, "--ignore-case": bashRONone,
			"--no-ignore-case": bashRONone,
			"-v": bashRONone, "--invert-match": bashRONone,
			"-w": bashRONone, "--word-regexp": bashRONone,
			"-x": bashRONone, "--line-regexp": bashRONone,
			"-c": bashRONone, "--count": bashRONone,
			"--color": bashROString, "--colour": bashROString,
			"-L": bashRONone, "--files-without-match": bashRONone,
			"-l": bashRONone, "--files-with-matches": bashRONone,
			"-m": bashRONumber, "--max-count": bashRONumber,
			"-o": bashRONone, "--only-matching": bashRONone,
			"-q": bashRONone, "--quiet": bashRONone, "--silent": bashRONone,
			"-s": bashRONone, "--no-messages": bashRONone,
			"-b": bashRONone, "--byte-offset": bashRONone,
			"-H": bashRONone, "--with-filename": bashRONone,
			"-h": bashRONone, "--no-filename": bashRONone,
			"--label": bashROString,
			"-n": bashRONone, "--line-number": bashRONone,
			"-T": bashRONone, "--initial-tab": bashRONone,
			"-u": bashRONone, "--unix-byte-offsets": bashRONone,
			"-Z": bashRONone, "--null": bashRONone,
			"-z": bashRONone, "--null-data": bashRONone,
			"-A": bashRONumber, "--after-context": bashRONumber,
			"-B": bashRONumber, "--before-context": bashRONumber,
			"-C": bashRONumber, "--context": bashRONumber,
			"--group-separator": bashROString,
			"--no-group-separator": bashRONone,
			"-a": bashRONone, "--text": bashRONone,
			"--binary-files": bashROString,
			"-D": bashROString, "--devices": bashROString,
			"-d": bashROString, "--directories": bashROString,
			"--exclude": bashROString, "--exclude-from": bashROString,
			"--exclude-dir": bashROString,
			"--include": bashROString,
			"-r": bashRONone, "--recursive": bashRONone,
			"-R": bashRONone, "--dereference-recursive": bashRONone,
			"--line-buffered": bashRONone,
			"-U": bashRONone, "--binary": bashRONone,
			"--help": bashRONone, "-V": bashRONone, "--version": bashRONone,
		},
	},

	// --- ripgrep ---
	"rg": {
		safeFlags: map[string]string{
			"-e": bashROString, "--regexp": bashROString,
			"-f": bashROString,
			"-i": bashRONone, "--ignore-case": bashRONone,
			"-S": bashRONone, "--smart-case": bashRONone,
			"-F": bashRONone, "--fixed-strings": bashRONone,
			"-w": bashRONone, "--word-regexp": bashRONone,
			"-v": bashRONone, "--invert-match": bashRONone,
			"-c": bashRONone, "--count": bashRONone,
			"-l": bashRONone, "--files-with-matches": bashRONone,
			"--files-without-match": bashRONone,
			"-n": bashRONone, "--line-number": bashRONone,
			"-o": bashRONone, "--only-matching": bashRONone,
			"-A": bashRONumber, "--after-context": bashRONumber,
			"-B": bashRONumber, "--before-context": bashRONumber,
			"-C": bashRONumber, "--context": bashRONumber,
			"-H": bashRONone, "-h": bashRONone,
			"--heading": bashRONone, "--no-heading": bashRONone,
			"-q": bashRONone, "--quiet": bashRONone,
			"--column": bashRONone,
			"-g": bashROString, "--glob": bashROString,
			"-t": bashROString, "--type": bashROString,
			"-T": bashROString, "--type-not": bashROString,
			"--type-list": bashRONone,
			"--hidden": bashRONone,
			"--no-ignore": bashRONone,
			"-u": bashRONone,
			"-m": bashRONumber, "--max-count": bashRONumber,
			"-d": bashRONumber, "--max-depth": bashRONumber,
			"-a": bashRONone, "--text": bashRONone,
			"-z": bashRONone,
			"-L": bashRONone, "--follow": bashRONone,
			"--color": bashROString,
			"--json": bashRONone,
			"--stats": bashRONone,
			"--help": bashRONone, "--version": bashRONone,
			"--debug": bashRONone,
		},
	},

	// --- sha256sum / sha1sum / md5sum ---
	"sha256sum": {
		safeFlags: map[string]string{
			"-b": bashRONone, "--binary": bashRONone,
			"-t": bashRONone, "--text": bashRONone,
			"-c": bashRONone, "--check": bashRONone,
			"--ignore-missing": bashRONone,
			"--quiet": bashRONone, "--status": bashRONone,
			"--strict": bashRONone,
			"-w": bashRONone, "--warn": bashRONone,
			"--tag": bashRONone,
			"-z": bashRONone, "--zero": bashRONone,
			"--help": bashRONone, "--version": bashRONone,
		},
	},
	"sha1sum": {
		safeFlags: map[string]string{
			"-b": bashRONone, "--binary": bashRONone,
			"-t": bashRONone, "--text": bashRONone,
			"-c": bashRONone, "--check": bashRONone,
			"--ignore-missing": bashRONone,
			"--quiet": bashRONone, "--status": bashRONone,
			"--strict": bashRONone,
			"-w": bashRONone, "--warn": bashRONone,
			"--tag": bashRONone,
			"-z": bashRONone, "--zero": bashRONone,
			"--help": bashRONone, "--version": bashRONone,
		},
	},
	"md5sum": {
		safeFlags: map[string]string{
			"-b": bashRONone, "--binary": bashRONone,
			"-t": bashRONone, "--text": bashRONone,
			"-c": bashRONone, "--check": bashRONone,
			"--ignore-missing": bashRONone,
			"--quiet": bashRONone, "--status": bashRONone,
			"--strict": bashRONone,
			"-w": bashRONone, "--warn": bashRONone,
			"--tag": bashRONone,
			"-z": bashRONone, "--zero": bashRONone,
			"--help": bashRONone, "--version": bashRONone,
		},
	},

	// --- tree ---
	"tree": {
		safeFlags: map[string]string{
			"-a": bashRONone, "-d": bashRONone, "-l": bashRONone,
			"-f": bashRONone, "-x": bashRONone,
			"-L": bashRONumber,
			"-P": bashROString, "-I": bashROString,
			"--gitignore": bashRONone, "--gitfile": bashROString,
			"--ignore-case": bashRONone, "--matchdirs": bashRONone,
			"--metafirst": bashRONone, "--prune": bashRONone,
			"--info": bashRONone, "--infofile": bashROString,
			"--noreport": bashRONone, "--charset": bashROString,
			"--filelimit": bashRONumber,
			"-q": bashRONone, "-N": bashRONone, "-Q": bashRONone,
			"-p": bashRONone, "-u": bashRONone, "-g": bashRONone,
			"-s": bashRONone, "-h": bashRONone,
			"--si": bashRONone, "--du": bashRONone,
			"-D": bashRONone, "--timefmt": bashROString,
			"-F": bashRONone, "--inodes": bashRONone,
			"--device": bashRONone,
			"-v": bashRONone, "-t": bashRONone,
			"-c": bashRONone, "-U": bashRONone, "-r": bashRONone,
			"--dirsfirst": bashRONone, "--filesfirst": bashRONone,
			"--sort": bashROString,
			"-i": bashRONone, "-A": bashRONone, "-S": bashRONone,
			"-C": bashRONone, "-X": bashRONone, "-J": bashRONone,
			"-H": bashROString,
			"--nolinks": bashRONone,
			"--hintro": bashROString, "--houtro": bashROString,
			"-T": bashROString, "--hyperlink": bashRONone,
			"--scheme": bashROString, "--authority": bashROString,
			"--fromfile": bashRONone, "--fromtabfile": bashRONone,
			"--fflinks": bashRONone,
			"--help": bashRONone, "--version": bashRONone,
		},
		// -R (recursive HTML write) and -o/--output (file write) excluded
	},

	// --- date ---
	"date": {
		safeFlags: map[string]string{
			"-d": bashROString, "--date": bashROString,
			"-r": bashROString, "--reference": bashROString,
			"-u": bashRONone, "--utc": bashRONone,
			"--universal": bashRONone,
			"-I": bashRONone, "--iso-8601": bashROString,
			"-R": bashRONone, "--rfc-email": bashRONone,
			"--rfc-3339": bashROString,
			"--debug": bashRONone, "--help": bashRONone,
			"--version": bashRONone,
		},
		// -s/--set and -f/--file excluded (can set system time)
			additionalDangerous: func(args []string, rawCmd string) bool {
				// Check for standalone positional args that look like time-setting
				// (MMDDhhmm format: all digits, 8+ chars). Flags like -d/--date
				// consume their arguments in validateBashROFlags, so we only need
				// to catch bare time-setting args like "date 01011200".
				for _, a := range args {
					stripped := bashROStripQuotes(a)
					if !strings.HasPrefix(stripped, "-") && !strings.HasPrefix(stripped, "+") {
						// Check if it looks like a time-setting arg (all digits, 8+ chars)
						if len(stripped) >= 8 {
							allDigits := true
							for _, c := range stripped {
								if c < '0' || c > '9' {
									allDigits = false
									break
								}
							}
							if allDigits {
								return true // Looks like MMDDhhmm time-setting
							}
						}
					}
				}
				return false
			},
	},

	// --- hostname ---
	"hostname": {
		safeFlags: map[string]string{
			"-f": bashRONone, "--fqdn": bashRONone, "--long": bashRONone,
			"-s": bashRONone, "--short": bashRONone,
			"-i": bashRONone, "--ip-address": bashRONone,
			"-I": bashRONone, "--all-ip-addresses": bashRONone,
			"-a": bashRONone, "--alias": bashRONone,
			"-d": bashRONone, "--domain": bashRONone,
			"-A": bashRONone, "--all-fqdns": bashRONone,
			"-v": bashRONone, "--verbose": bashRONone,
			"-h": bashRONone, "--help": bashRONone,
			"-V": bashRONone, "--version": bashRONone,
		},
		// NO positional args allowed — can set hostname
		additionalDangerous: func(args []string, rawCmd string) bool {
			for _, a := range args {
				if !strings.HasPrefix(a, "-") {
					return true // positional = hostname change attempt
				}
			}
			return false
		},
	},

	// --- info ---
	"info": {
		safeFlags: map[string]string{
			"-f": bashROString, "--file": bashROString,
			"-d": bashROString, "--directory": bashROString,
			"-n": bashROString, "--node": bashROString,
			"-a": bashRONone, "--all": bashRONone,
			"-k": bashROString, "--apropos": bashROString,
			"-w": bashRONone, "--where": bashRONone,
			"--location": bashRONone, "--show-options": bashRONone,
			"--vi-keys": bashRONone, "--subnodes": bashRONone,
			"-h": bashRONone, "--help": bashRONone,
			"--usage": bashRONone, "--version": bashRONone,
		},
		// -o/--output, --dribble, --init-file, --restore excluded
	},

	// --- lsof ---
	"lsof": {
		safeFlags: map[string]string{
			"-?": bashRONone, "-h": bashRONone, "-v": bashRONone,
			"-a": bashRONone, "-b": bashRONone,
			"-C": bashRONone, "-l": bashRONone,
			"-n": bashRONone, "-N": bashRONone,
			"-O": bashRONone, "-P": bashRONone,
			"-Q": bashRONone, "-R": bashRONone,
			"-t": bashRONone, "-U": bashRONone,
			"-V": bashRONone, "-X": bashRONone,
			"-H": bashRONone, "-E": bashRONone,
			"-F": bashRONone, "-g": bashRONone,
			"-i": bashRONone, "-K": bashRONone,
			"-L": bashRONone, "-o": bashRONone,
			"-r": bashRONone, "-s": bashRONone,
			"-S": bashRONone, "-T": bashRONone,
			"-x": bashRONone,
			"-A": bashROString, "-c": bashROString,
			"-d": bashROString, "-e": bashROString,
			"-k": bashROString, "-p": bashROString,
			"-u": bashROString,
		},
		// +m (creates mount supplement file) excluded
		// -D (device cache write) excluded
		additionalDangerous: func(args []string, rawCmd string) bool {
			for _, a := range args {
				if a == "+m" || strings.HasPrefix(a, "+m") {
					return true
				}
			}
			return false
		},
	},

	// --- pgrep ---
	"pgrep": {
		safeFlags: map[string]string{
			"-d": bashROString, "--delimiter": bashROString,
			"-l": bashRONone, "--list-name": bashRONone,
			"-a": bashRONone, "--list-full": bashRONone,
			"-v": bashRONone, "--inverse": bashRONone,
			"-w": bashRONone, "--lightweight": bashRONone,
			"-c": bashRONone, "--count": bashRONone,
			"-f": bashRONone, "--full": bashRONone,
			"-g": bashROString, "--pgroup": bashROString,
			"-G": bashROString, "--group": bashROString,
			"-i": bashRONone, "--ignore-case": bashRONone,
			"-n": bashRONone, "--newest": bashRONone,
			"-o": bashRONone, "--oldest": bashRONone,
			"-O": bashROString, "--older": bashROString,
			"-P": bashROString, "--parent": bashROString,
			"-s": bashROString, "--session": bashROString,
			"-t": bashROString, "--terminal": bashROString,
			"-u": bashROString, "--euid": bashROString,
			"-U": bashROString, "--uid": bashROString,
			"-x": bashRONone, "--exact": bashRONone,
			"-F": bashROString, "--pidfile": bashROString,
			"-L": bashRONone, "--logpidfile": bashRONone,
			"-r": bashROString, "--runstates": bashROString,
			"--ns": bashROString, "--nslist": bashROString,
			"--help": bashRONone,
			"-V": bashRONone, "--version": bashRONone,
		},
	},

	// --- tput ---
	"tput": {
		safeFlags: map[string]string{
			"-T": bashROString,
			"-V": bashRONone,
			"-x": bashRONone,
		},
		// Blocks dangerous capabilities and -S (stdin input)
		additionalDangerous: func(args []string, rawCmd string) bool {
			dangerousCaps := map[string]bool{
				"init": true, "reset": true, "rs1": true, "rs2": true,
				"rs3": true, "is1": true, "is2": true, "is3": true,
				"iprog": true, "if": true, "rf": true, "clear": true,
				"flash": true, "mc0": true, "mc4": true, "mc5": true,
				"mc5i": true, "mc5p": true, "pfkey": true, "pfloc": true,
				"pfx": true, "pfxl": true, "smcup": true, "rmcup": true,
			}
			for _, a := range args {
				if a == "-S" {
					return true
				}
				if dangerousCaps[strings.ToLower(a)] {
					return true
				}
			}
			return false
		},
	},

	// --- ss ---
	"ss": {
		safeFlags: map[string]string{
			"-h": bashRONone, "--help": bashRONone,
			"-V": bashRONone, "--version": bashRONone,
			"-n": bashRONone, "--numeric": bashRONone,
			"-r": bashRONone, "--resolve": bashRONone,
			"-a": bashRONone, "--all": bashRONone,
			"-l": bashRONone, "--listening": bashRONone,
			"-o": bashRONone, "--options": bashRONone,
			"-e": bashRONone, "--extended": bashRONone,
			"-m": bashRONone, "--memory": bashRONone,
			"-p": bashRONone, "--processes": bashRONone,
			"-i": bashRONone, "--info": bashRONone,
			"-s": bashRONone, "--summary": bashRONone,
			"-4": bashRONone, "--ipv4": bashRONone,
			"-6": bashRONone, "--ipv6": bashRONone,
			"-0": bashRONone, "--packet": bashRONone,
			"-t": bashRONone, "--tcp": bashRONone,
			"-M": bashRONone, "--mptcp": bashRONone,
			"-S": bashRONone, "--sctp": bashRONone,
			"-u": bashRONone, "--udp": bashRONone,
			"-d": bashRONone, "--dccp": bashRONone,
			"-w": bashRONone, "--raw": bashRONone,
			"-x": bashRONone, "--unix": bashRONone,
			"--tipc": bashRONone, "--vsock": bashRONone,
			"-f": bashROString, "--family": bashROString,
			"-A": bashROString, "--query": bashROString,
			"--socket": bashROString,
			"-Z": bashRONone, "--context": bashRONone,
			"-z": bashRONone, "--contexts": bashRONone,
			"-b": bashRONone, "--bpf": bashRONone,
			"-E": bashRONone, "--events": bashRONone,
			"-H": bashRONone, "--no-header": bashRONone,
			"-O": bashRONone, "--oneline": bashRONone,
			"--tipcinfo": bashRONone, "--tos": bashRONone,
			"--cgroup": bashRONone, "--inet-sockopt": bashRONone,
		},
		// -K/--kill, -D/--diag, -F/--filter, -N/--net excluded
	},
}

// ===========================================================================
// validateBashROFlags — validates flags for a read-only command
// ===========================================================================

// validateBashROFlags validates that all flags in a command are in the
// safe-flags whitelist for that command. Returns true if safe, false if dangerous.
func validateBashROFlags(cmd string, config *bashROFlagConfig) bool {
	if config == nil {
		return false
	}
	if config.additionalDangerous != nil {
		fields := strings.Fields(cmd)
		// Skip the first token (command name)
		if len(fields) > 1 && config.additionalDangerous(fields[1:], cmd) {
			return false
		}
	}

	fields := strings.Fields(cmd)
	if len(fields) <= 1 {
		return true // just the command name, no args
	}
	args := fields[1:]

	respectsDD := true
	if config.respectsDoubleDash == false {
		respectsDD = false
	}

	seenDD := false
	for i := 0; i < len(args); i++ {
		token := args[i]
		if token == "" {
			continue
		}
		if token == "--" {
			if respectsDD {
				break // POSIX end-of-options
			}
			seenDD = true
			continue
		}

		if !strings.HasPrefix(token, "-") && !seenDD {
			continue // positional arg, allowed
		}
		if !strings.HasPrefix(token, "-") && seenDD {
			continue // after --, positional args allowed
		}
		if len(token) < 2 {
			continue // just "-" by itself (stdin)
		}

		// Check for dangerous flags first
		if config.dangerousFlags[token] {
			return false
		}

		// Handle numeric shorthand like -A20 (attached numeric arg)
		if len(token) > 1 && token[0] == '-' && token[1] >= '0' && token[1] <= '9' {
			// For commands that commonly use attached numeric args
			if cmdType := getCmdTypeForAttachedNumbers(cmd); cmdType != "" {
				continue
			}
		}

		// Parse flag
		hasEquals := strings.Contains(token, "=")
		var flag string
		var inlineValue string
		if hasEquals {
			parts := strings.SplitN(token, "=", 2)
			flag = parts[0]
			inlineValue = parts[1]
		} else {
			flag = token
		}

		argType, ok := config.safeFlags[flag]
		if !ok {
			// Single short flag: -R
			if len(flag) == 2 && flag[0] == '-' {
				return false
			}
			// Could be short flag with attached arg (-n5) or combined flags (-la)
			if len(flag) > 2 && flag[0] == '-' && flag[1] != '-' {
				// Try as short flag with attached arg first
				singleFlag := "-" + string(flag[1])
				if st, found := config.safeFlags[singleFlag]; found && st != bashRONone {
					singleArg := flag[2:]
					if bashROValidateFlagArgTyped(singleArg, st) {
						continue
					}
				}
				// Combined short flags: all must be 'none' type
				for j := 1; j < len(flag); j++ {
					singleFlag := "-" + string(flag[j])
					ft, found := config.safeFlags[singleFlag]
					if !found || ft != bashRONone {
						return false
					}
				}
				continue
			}
			return false
		}

		if argType == bashRONone {
			if hasEquals {
				return false // none-type flag should not have value
			}
			continue
		}

		// Flag takes an argument
		var argValue string
		if hasEquals {
			argValue = bashROStripQuotes(inlineValue)
		} else {
			if i+1 >= len(args) {
				return false // Missing required argument
			}
			argValue = bashROStripQuotes(args[i+1])
			i++ // consume next arg
		}

		if !bashROValidateFlagArgTyped(argValue, argType) {
			return false
		}
	}
	return true
}

// bashROStripQuotes removes matching leading/trailing single or double quotes from a token.
func bashROStripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// bashROValidateFlagArgTyped validates a flag argument based on its expected type.
func bashROValidateFlagArgTyped(value string, argType string) bool {
	switch argType {
	case bashRONone:
		return false // shouldn't be called
	case bashRONumber:
		if len(value) == 0 {
			return false
		}
		for _, c := range value {
			if c < '0' || c > '9' {
				return false
			}
		}
		return true
	case bashROString:
		return true // any string
	case bashROChar:
		return len(value) == 1
	case bashROBrace:
		return value == "{}"
	case bashROEOF:
		return value == "EOF"
	default:
		return false
	}
}

// getCmdTypeForAttachedNumbers returns the command type for attached numeric arg handling.
func getCmdTypeForAttachedNumbers(cmd string) string {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// ===========================================================================
// checkBashReadOnlyCommand — validates a command against the read-only allowlist
// ===========================================================================

// checkBashReadOnlyCommand checks if a command is a known read-only command
// with safe flags. Returns true if read-only, false if unknown or dangerous.
func checkBashReadOnlyCommand(cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return false
	}
	fields := strings.Fields(trimmed)
	bin := strings.ToLower(fields[0])

	config, ok := bashReadOnlyCommands[bin]
	if !ok {
		return false
	}

	return validateBashROFlags(trimmed, config)
}
