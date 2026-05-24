package tools

import (
	"strings"
)

// ===========================================================================
// Git read-only command validation for bash (upstream readOnlyCommandValidation.ts)
// Used by CheckBashPermission for `git` commands run via bash/shell.
// Renamed with bashRO prefix to avoid conflicts with git_tool.go's GitTool system.
// ===========================================================================

// bashROFlagArgType represents the type of argument a flag accepts.
type bashROFlagArgType string

const (
	bashROFlagNone   bashROFlagArgType = "none"   // No argument (--color, -n)
	bashROFlagNumber bashROFlagArgType = "number" // Integer argument (--context=3)
	bashROFlagString bashROFlagArgType = "string" // Any string argument (--relative=path)
	bashROFlagChar   bashROFlagArgType = "char"   // Single character (delimiter)
	bashROFlagBrace  bashROFlagArgType = "{}"     // Literal "{}" only
	bashROFlagEOF    bashROFlagArgType = "EOF"    // Literal "EOF" only
)

// bashROCommandConfig represents the configuration for a git subcommand's
// read-only flag validation.
type bashROCommandConfig struct {
	safeFlags         map[string]bashROFlagArgType
	dangerousCallback func(args []string) bool // returns true if dangerous
}

// bashROGitCommands maps git subcommands to their read-only flag configs.
var bashROGitCommands = map[string]*bashROCommandConfig{
	"git diff": &bashROCommandConfig{
		safeFlags: bashROGitDiffFlags,
	},
	"git log": &bashROCommandConfig{
		safeFlags: bashROGitLogFlags,
	},
	"git show": &bashROCommandConfig{
		safeFlags: bashROGitShowFlags,
	},
	"git status": &bashROCommandConfig{
		safeFlags: bashROGitStatusFlags,
	},
	"git blame": &bashROCommandConfig{
		safeFlags: bashROGitBlameFlags,
	},
	"git ls-files": &bashROCommandConfig{
		safeFlags: bashROGitLsFilesFlags,
	},
	"git branch": &bashROCommandConfig{
		safeFlags:         bashROGitBranchFlags,
		dangerousCallback: bashROGitBranchIsDangerous,
	},
	"git tag": &bashROCommandConfig{
		safeFlags:         bashROGitTagFlags,
		dangerousCallback: bashROGitTagIsDangerous,
	},
	"git reflog": &bashROCommandConfig{
		safeFlags:         bashROGitReflogFlags,
		dangerousCallback: bashROGitReflogIsDangerous,
	},
	"git remote show": &bashROCommandConfig{
		safeFlags:         map[string]bashROFlagArgType{"-n": bashROFlagNone},
		dangerousCallback: bashROGitRemoteShowIsDangerous,
	},
	"git remote": &bashROCommandConfig{
		safeFlags:         map[string]bashROFlagArgType{"-v": bashROFlagNone, "--verbose": bashROFlagNone},
		dangerousCallback: bashROGitRemoteIsDangerous,
	},
	"git config --get": &bashROCommandConfig{
		safeFlags: bashROGitConfigGetFlags,
	},
	"git rev-parse": &bashROCommandConfig{
		safeFlags: bashROGitRevParseFlags,
	},
	"git describe": &bashROCommandConfig{
		safeFlags: bashROGitDescribeFlags,
	},
	"git ls-remote": &bashROCommandConfig{
		safeFlags: bashROGitLsRemoteFlags,
	},
	"git shortlog": &bashROCommandConfig{
		safeFlags: bashROGitShortlogFlags,
	},
	"git stash list": &bashROCommandConfig{
		safeFlags: bashROGitStashListFlags,
	},
	"git stash show": &bashROCommandConfig{
		safeFlags: bashROGitStashShowFlags,
	},
	"git merge-base": &bashROCommandConfig{
		safeFlags: bashROGitMergeBaseFlags,
	},
	"git for-each-ref": &bashROCommandConfig{
		safeFlags: bashROGitForEachRefFlags,
	},
	"git grep": &bashROCommandConfig{
		safeFlags: bashROGitGrepFlags,
	},
	"git worktree list": &bashROCommandConfig{
		safeFlags: bashROGitWorktreeListFlags,
	},
}

// ===========================================================================
// Git flag groups (shared across subcommands)
// ===========================================================================

var bashROGitRefSelectionFlags = map[string]bashROFlagArgType{
	"--all": bashROFlagNone, "--branches": bashROFlagNone,
	"--tags": bashROFlagNone, "--remotes": bashROFlagNone,
}

var bashROGitDateFilterFlags = map[string]bashROFlagArgType{
	"--since": bashROFlagString, "--after": bashROFlagString,
	"--until": bashROFlagString, "--before": bashROFlagString,
}

var bashROGitLogDisplayFlags = map[string]bashROFlagArgType{
	"--oneline": bashROFlagNone, "--graph": bashROFlagNone,
	"--decorate": bashROFlagNone, "--no-decorate": bashROFlagNone,
	"--date": bashROFlagString, "--relative-date": bashROFlagNone,
}

var bashROGitCountFlags = map[string]bashROFlagArgType{
	"--max-count": bashROFlagNumber, "-n": bashROFlagNumber,
}

var bashROGitStatFlags = map[string]bashROFlagArgType{
	"--stat": bashROFlagNone, "--numstat": bashROFlagNone,
	"--shortstat": bashROFlagNone, "--name-only": bashROFlagNone,
	"--name-status": bashROFlagNone,
}

var bashROGitColorFlags = map[string]bashROFlagArgType{
	"--color": bashROFlagNone, "--no-color": bashROFlagNone,
}

var bashROGitPatchFlags = map[string]bashROFlagArgType{
	"--patch": bashROFlagNone, "-p": bashROFlagNone,
	"--no-patch": bashROFlagNone, "--no-ext-diff": bashROFlagNone,
	"-s": bashROFlagNone,
}

var bashROGitAuthorFilterFlags = map[string]bashROFlagArgType{
	"--author": bashROFlagString, "--committer": bashROFlagString,
	"--grep": bashROFlagString,
}

// bashROMergeGitFlags merges multiple flag groups into one map.
func bashROMergeGitFlags(groups ...map[string]bashROFlagArgType) map[string]bashROFlagArgType {
	result := make(map[string]bashROFlagArgType, 0)
	for _, g := range groups {
		for k, v := range g {
			result[k] = v
		}
	}
	return result
}

// ===========================================================================
// Individual git subcommand flag maps
// ===========================================================================

var bashROGitDiffFlags = bashROMergeGitFlags(bashROGitStatFlags, bashROGitColorFlags, map[string]bashROFlagArgType{
	"--dirstat": bashROFlagNone, "--summary": bashROFlagNone, "--patch-with-stat": bashROFlagNone,
	"--word-diff": bashROFlagNone, "--word-diff-regex": bashROFlagString,
	"--color-words": bashROFlagNone, "--no-renames": bashROFlagNone,
	"--check": bashROFlagNone, "--full-index": bashROFlagNone,
	"--binary": bashROFlagNone, "--abbrev": bashROFlagNumber,
	"--exit-code": bashROFlagNone, "--quiet": bashROFlagNone,
	"--cached": bashROFlagNone, "--staged": bashROFlagNone,
	"--diff-filter": bashROFlagString, "--relative": bashROFlagString,
	"-M": bashROFlagNone, "-C": bashROFlagNone, "-B": bashROFlagNone, "-R": bashROFlagNone,
	"-S": bashROFlagString, "-G": bashROFlagString, "-O": bashROFlagString,
})

var bashROGitLogFlags = bashROMergeGitFlags(bashROGitLogDisplayFlags, bashROGitRefSelectionFlags, bashROGitDateFilterFlags,
	bashROGitCountFlags, bashROGitStatFlags, bashROGitColorFlags, bashROGitPatchFlags, bashROGitAuthorFilterFlags,
	map[string]bashROFlagArgType{
		"--abbrev-commit": bashROFlagNone, "--full-history": bashROFlagNone,
		"--dense": bashROFlagNone, "--sparse": bashROFlagNone,
		"--simplify-merges": bashROFlagNone, "--ancestry-path": bashROFlagNone,
		"--source": bashROFlagNone, "--first-parent": bashROFlagNone,
		"--merges": bashROFlagNone, "--no-merges": bashROFlagNone,
		"--reverse": bashROFlagNone, "--walk-reflogs": bashROFlagNone,
		"--skip": bashROFlagNumber, "--no-walk": bashROFlagNone,
		"--pretty": bashROFlagString, "--format": bashROFlagString,
		"--diff-filter": bashROFlagString,
		"-S": bashROFlagString, "-G": bashROFlagString,
		"--pickaxe-regex": bashROFlagNone, "--pickaxe-all": bashROFlagNone,
	})

var bashROGitShowFlags = bashROMergeGitFlags(bashROGitLogDisplayFlags, bashROGitStatFlags, bashROGitColorFlags,
	bashROGitPatchFlags, map[string]bashROFlagArgType{
		"--abbrev-commit": bashROFlagNone, "--word-diff": bashROFlagNone,
		"--pretty": bashROFlagString, "--format": bashROFlagString,
		"--first-parent": bashROFlagNone, "--raw": bashROFlagNone,
		"--diff-filter": bashROFlagString, "-m": bashROFlagNone, "--quiet": bashROFlagNone,
	})

var bashROGitStatusFlags = map[string]bashROFlagArgType{
	"--short": bashROFlagNone, "-s": bashROFlagNone,
	"--branch": bashROFlagNone, "-b": bashROFlagNone,
	"--porcelain": bashROFlagNone, "--long": bashROFlagNone,
	"--verbose": bashROFlagNone, "-v": bashROFlagNone,
	"--untracked-files": bashROFlagString, "-u": bashROFlagString,
	"--ignored": bashROFlagNone, "--ignore-submodules": bashROFlagString,
	"--column": bashROFlagNone, "--no-column": bashROFlagNone,
	"--ahead-behind": bashROFlagNone, "--no-ahead-behind": bashROFlagNone,
	"--renames": bashROFlagNone, "--no-renames": bashROFlagNone,
	"--find-renames": bashROFlagString, "-M": bashROFlagString,
}

var bashROGitBlameFlags = bashROMergeGitFlags(bashROGitColorFlags, map[string]bashROFlagArgType{
	"-L": bashROFlagString, "--porcelain": bashROFlagNone, "-p": bashROFlagNone,
	"--line-porcelain": bashROFlagNone, "--incremental": bashROFlagNone,
	"--root": bashROFlagNone, "--show-stats": bashROFlagNone,
	"--show-name": bashROFlagNone, "--show-number": bashROFlagNone, "-n": bashROFlagNone,
	"--show-email": bashROFlagNone, "-e": bashROFlagNone, "-f": bashROFlagNone,
	"--date": bashROFlagString, "-w": bashROFlagNone,
	"--ignore-rev": bashROFlagString, "--ignore-revs-file": bashROFlagString,
	"-M": bashROFlagNone, "-C": bashROFlagNone, "--score-debug": bashROFlagNone,
	"--abbrev": bashROFlagNumber, "-s": bashROFlagNone, "-l": bashROFlagNone, "-t": bashROFlagNone,
})

var bashROGitLsFilesFlags = map[string]bashROFlagArgType{
	"--cached": bashROFlagNone, "-c": bashROFlagNone,
	"--deleted": bashROFlagNone, "-d": bashROFlagNone,
	"--modified": bashROFlagNone, "-m": bashROFlagNone,
	"--others": bashROFlagNone, "-o": bashROFlagNone,
	"--ignored": bashROFlagNone, "-i": bashROFlagNone,
	"--stage": bashROFlagNone, "-s": bashROFlagNone,
	"--killed": bashROFlagNone, "-k": bashROFlagNone,
	"--unmerged": bashROFlagNone, "-u": bashROFlagNone,
	"--directory": bashROFlagNone, "--eol": bashROFlagNone,
	"--full-name": bashROFlagNone, "--abbrev": bashROFlagNumber,
	"-z": bashROFlagNone, "-t": bashROFlagNone, "-v": bashROFlagNone, "-f": bashROFlagNone,
	"--exclude": bashROFlagString, "-x": bashROFlagString,
	"--exclude-from": bashROFlagString, "-X": bashROFlagString,
	"--exclude-standard": bashROFlagNone,
	"--error-unmatch": bashROFlagNone, "--recurse-submodules": bashROFlagNone,
}

var bashROGitBranchFlags = map[string]bashROFlagArgType{
	"-l": bashROFlagNone, "--list": bashROFlagNone,
	"-a": bashROFlagNone, "--all": bashROFlagNone,
	"-r": bashROFlagNone, "--remotes": bashROFlagNone,
	"-v": bashROFlagNone, "-vv": bashROFlagNone, "--verbose": bashROFlagNone,
	"--color": bashROFlagNone, "--no-color": bashROFlagNone,
	"--column": bashROFlagNone, "--no-column": bashROFlagNone,
	"--abbrev": bashROFlagNumber, "--no-abbrev": bashROFlagNone,
	"--contains": bashROFlagString, "--no-contains": bashROFlagString,
	"--merged": bashROFlagNone, "--no-merged": bashROFlagNone,
	"--points-at": bashROFlagString, "--sort": bashROFlagString,
	"--show-current": bashROFlagNone,
	"-i": bashROFlagNone, "--ignore-case": bashROFlagNone,
}

var bashROGitTagFlags = map[string]bashROFlagArgType{
	"-l": bashROFlagNone, "--list": bashROFlagNone,
	"-n": bashROFlagNumber, "--contains": bashROFlagString,
	"--no-contains": bashROFlagString,
	"--merged": bashROFlagString, "--no-merged": bashROFlagString,
	"--sort": bashROFlagString, "--format": bashROFlagString,
	"--points-at": bashROFlagString,
	"--column": bashROFlagNone, "--no-column": bashROFlagNone,
	"-i": bashROFlagNone, "--ignore-case": bashROFlagNone,
}

var bashROGitReflogFlags = bashROMergeGitFlags(bashROGitLogDisplayFlags, bashROGitRefSelectionFlags,
	bashROGitDateFilterFlags, bashROGitCountFlags, bashROGitAuthorFilterFlags)

var bashROGitConfigGetFlags = map[string]bashROFlagArgType{
	"--local": bashROFlagNone, "--global": bashROFlagNone,
	"--system": bashROFlagNone, "--worktree": bashROFlagNone,
	"--default": bashROFlagString, "--type": bashROFlagString,
	"--bool": bashROFlagNone, "--int": bashROFlagNone,
	"--bool-or-int": bashROFlagNone, "--path": bashROFlagNone,
	"--expiry-date": bashROFlagNone, "-z": bashROFlagNone, "--null": bashROFlagNone,
	"--name-only": bashROFlagNone, "--show-origin": bashROFlagNone,
	"--show-scope": bashROFlagNone,
}

var bashROGitRevParseFlags = map[string]bashROFlagArgType{
	"--verify": bashROFlagNone, "--short": bashROFlagString,
	"--abbrev-ref": bashROFlagNone, "--symbolic": bashROFlagNone,
	"--symbolic-full-name": bashROFlagNone,
	"--show-toplevel": bashROFlagNone, "--show-cdup": bashROFlagNone,
	"--show-prefix": bashROFlagNone, "--git-dir": bashROFlagNone,
	"--git-common-dir": bashROFlagNone, "--absolute-git-dir": bashROFlagNone,
	"--is-inside-work-tree": bashROFlagNone, "--is-inside-git-dir": bashROFlagNone,
	"--is-bare-repository": bashROFlagNone, "--is-shallow-repository": bashROFlagNone,
}

var bashROGitDescribeFlags = map[string]bashROFlagArgType{
	"--tags": bashROFlagNone, "--match": bashROFlagString,
	"--exclude": bashROFlagString, "--long": bashROFlagNone,
	"--abbrev": bashROFlagNumber, "--always": bashROFlagNone,
	"--contains": bashROFlagNone, "--first-match": bashROFlagNone,
	"--exact-match": bashROFlagNone, "--candidates": bashROFlagNumber,
	"--dirty": bashROFlagNone, "--broken": bashROFlagNone,
}

var bashROGitLsRemoteFlags = map[string]bashROFlagArgType{
	"--branches": bashROFlagNone, "-b": bashROFlagNone,
	"--tags": bashROFlagNone, "-t": bashROFlagNone,
	"--heads": bashROFlagNone, "-h": bashROFlagNone,
	"--refs": bashROFlagNone, "--quiet": bashROFlagNone, "-q": bashROFlagNone,
	"--exit-code": bashROFlagNone, "--get-url": bashROFlagNone,
	"--symref": bashROFlagNone, "--sort": bashROFlagString,
}

var bashROGitShortlogFlags = bashROMergeGitFlags(bashROGitRefSelectionFlags, bashROGitDateFilterFlags,
	map[string]bashROFlagArgType{
		"-s": bashROFlagNone, "--summary": bashROFlagNone,
		"-n": bashROFlagNone, "--numbered": bashROFlagNone,
		"-e": bashROFlagNone, "--email": bashROFlagNone,
		"-c": bashROFlagNone, "--committer": bashROFlagNone,
		"--group": bashROFlagString, "--format": bashROFlagString,
		"--no-merges": bashROFlagNone, "--author": bashROFlagString,
	})

var bashROGitStashListFlags = bashROMergeGitFlags(bashROGitLogDisplayFlags, bashROGitRefSelectionFlags, bashROGitCountFlags)

var bashROGitStashShowFlags = bashROMergeGitFlags(bashROGitStatFlags, bashROGitColorFlags, bashROGitPatchFlags,
	map[string]bashROFlagArgType{
		"--word-diff": bashROFlagNone, "--diff-filter": bashROFlagString,
		"--abbrev": bashROFlagNumber,
	})

var bashROGitMergeBaseFlags = map[string]bashROFlagArgType{
	"--is-ancestor": bashROFlagNone, "--fork-point": bashROFlagNone,
	"--octopus": bashROFlagNone, "--independent": bashROFlagNone,
	"--all": bashROFlagNone,
}

var bashROGitForEachRefFlags = map[string]bashROFlagArgType{
	"--format": bashROFlagString, "--sort": bashROFlagString,
	"--count": bashROFlagNumber,
	"--contains": bashROFlagString, "--no-contains": bashROFlagString,
	"--merged": bashROFlagString, "--no-merged": bashROFlagString,
	"--points-at": bashROFlagString,
}

var bashROGitGrepFlags = map[string]bashROFlagArgType{
	"-e": bashROFlagString, "-E": bashROFlagNone, "--extended-regexp": bashROFlagNone,
	"-G": bashROFlagNone, "--basic-regexp": bashROFlagNone,
	"-F": bashROFlagNone, "--fixed-strings": bashROFlagNone,
	"-P": bashROFlagNone, "--perl-regexp": bashROFlagNone,
	"-i": bashROFlagNone, "--ignore-case": bashROFlagNone,
	"-v": bashROFlagNone, "--invert-match": bashROFlagNone,
	"-w": bashROFlagNone, "--word-regexp": bashROFlagNone,
	"-n": bashROFlagNone, "--line-number": bashROFlagNone,
	"-c": bashROFlagNone, "--count": bashROFlagNone,
	"-l": bashROFlagNone, "--files-with-matches": bashROFlagNone,
	"-h": bashROFlagNone, "-H": bashROFlagNone,
	"-o": bashROFlagNone, "--only-matching": bashROFlagNone,
	"-A": bashROFlagNumber, "--after-context": bashROFlagNumber,
	"-B": bashROFlagNumber, "--before-context": bashROFlagNumber,
	"-C": bashROFlagNumber, "--context": bashROFlagNumber,
	"--max-depth": bashROFlagNumber, "--untracked": bashROFlagNone,
	"--no-index": bashROFlagNone, "--cached": bashROFlagNone,
	"--threads": bashROFlagNumber, "-q": bashROFlagNone,
}

var bashROGitWorktreeListFlags = map[string]bashROFlagArgType{
	"--porcelain": bashROFlagNone, "-v": bashROFlagNone, "--verbose": bashROFlagNone,
	"--expire": bashROFlagString,
}

// ===========================================================================
// Git dangerous callbacks (additionalCommandIsDangerousCallback equivalents)
// ===========================================================================

// bashROGitBranchIsDangerous blocks branch creation via positional arguments.
func bashROGitBranchIsDangerous(args []string) bool {
	flagsWithArgs := map[string]bool{
		"--contains": true, "--no-contains": true,
		"--points-at": true, "--sort": true,
	}
	var seenListFlag bool
	var seenDashDash bool
	for i := 0; i < len(args); i++ {
		token := args[i]
		if token == "" {
			continue
		}
		if token == "--" && !seenDashDash {
			seenDashDash = true
			continue
		}
		if !seenDashDash && strings.HasPrefix(token, "-") {
			if token == "--list" || token == "-l" {
				seenListFlag = true
			} else if len(token) > 2 && token[0] == '-' && token[1] != '-' &&
				!strings.Contains(token, "=") && strings.Contains(token[1:], "l") {
				seenListFlag = true
			}
			if strings.Contains(token, "=") {
				continue
			} else if flagsWithArgs[token] {
				i++
				continue
			}
			continue
		}
		if !seenListFlag {
			return true
		}
	}
	return false
}

// bashROGitTagIsDangerous blocks tag creation via positional arguments.
func bashROGitTagIsDangerous(args []string) bool {
	flagsWithArgs := map[string]bool{
		"--contains": true, "--no-contains": true,
		"--merged": true, "--no-merged": true,
		"--points-at": true, "--sort": true, "--format": true,
		"-n": true,
	}
	var seenListFlag bool
	var seenDashDash bool
	for i := 0; i < len(args); i++ {
		token := args[i]
		if token == "" {
			continue
		}
		if token == "--" && !seenDashDash {
			seenDashDash = true
			continue
		}
		if !seenDashDash && strings.HasPrefix(token, "-") {
			if token == "--list" || token == "-l" {
				seenListFlag = true
			} else if len(token) > 2 && token[0] == '-' && token[1] != '-' &&
				!strings.Contains(token, "=") && strings.Contains(token[1:], "l") {
				seenListFlag = true
			}
			if strings.Contains(token, "=") {
				continue
			} else if flagsWithArgs[token] {
				i++
				continue
			}
			continue
		}
		if !seenListFlag {
			return true
		}
	}
	return false
}

// bashROGitReflogIsDangerous blocks expire/delete/exists subcommands.
func bashROGitReflogIsDangerous(args []string) bool {
	dangerousSubcommands := map[string]bool{
		"expire": true, "delete": true, "exists": true,
	}
	for _, token := range args {
		if token == "" || strings.HasPrefix(token, "-") {
			continue
		}
		if dangerousSubcommands[token] {
			return true
		}
		return false
	}
	return false
}

// bashROGitRemoteShowIsDangerous validates "git remote show".
func bashROGitRemoteShowIsDangerous(args []string) bool {
	var positional []string
	for _, a := range args {
		if a != "-n" {
			positional = append(positional, a)
		}
	}
	if len(positional) != 1 {
		return true
	}
	for _, c := range positional[0] {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-') {
			return true
		}
	}
	return false
}

// bashROGitRemoteIsDangerous only allows bare "git remote" or "-v".
func bashROGitRemoteIsDangerous(args []string) bool {
	for _, a := range args {
		if a != "-v" && a != "--verbose" {
			return true
		}
	}
	return false
}

// ===========================================================================
// Git flag validation logic
// ===========================================================================

// bashROValidateGitFlags validates flags for a git subcommand against its config.
func bashROValidateGitFlags(config *bashROCommandConfig, args []string) bool {
	if config == nil {
		return false
	}
	if config.dangerousCallback != nil && config.dangerousCallback(args) {
		return false
	}
	for i := 0; i < len(args); i++ {
		token := args[i]
		if token == "" {
			continue
		}
		if token == "--" {
			break
		}
		if strings.HasPrefix(token, "-") && len(token) > 1 {
			// Handle numeric shorthand like -20, -5 (git log/show/reflog)
			// These are equivalent to -n N or --max-count=N
			if token[1] >= '0' && token[1] <= '9' {
				continue // Numeric shorthand is always safe
			}

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
				// Short flag with attached argument: -n5 (same as -n 5)
				// Try extracting the first char after '-' as a flag that takes an arg
				if len(flag) > 2 && flag[0] == '-' && flag[1] != '-' {
					singleFlag := "-" + string(flag[1])
					singleArg := flag[2:]
					if st, found := config.safeFlags[singleFlag]; found && st != bashROFlagNone {
						if bashROValidateFlagArg(singleArg, st) {
							continue
						}
						return false
					}
					// Combined short flags: all must be 'none' type
					for j := 1; j < len(flag); j++ {
						singleFlag := "-" + string(flag[j])
						ft, found := config.safeFlags[singleFlag]
						if !found || ft != bashROFlagNone {
							return false
						}
					}
					continue
				}
				return false
			}

			if argType == bashROFlagNone {
				if hasEquals {
					return false
				}
				continue
			}

			var argValue string
			if hasEquals {
				argValue = inlineValue
			} else {
				if i+1 >= len(args) {
					return false
				}
				argValue = args[i+1]
				i++
			}

			if !bashROValidateFlagArg(argValue, argType) {
				return false
			}
		}
	}
	return true
}

// bashROValidateFlagArg validates a flag argument value based on its expected type.
func bashROValidateFlagArg(value string, argType bashROFlagArgType) bool {
	switch argType {
	case bashROFlagNone:
		return false
	case bashROFlagNumber:
		for _, c := range value {
			if c < '0' || c > '9' {
				return false
			}
		}
		return len(value) > 0
	case bashROFlagString:
		return true
	case bashROFlagChar:
		return len(value) == 1
	case bashROFlagBrace:
		return value == "{}"
	case bashROFlagEOF:
		return value == "EOF"
	default:
		return false
	}
}

// ===========================================================================
// bashROIsGitReadOnlyCommand — checks if a git command is read-only
// ===========================================================================

// bashROIsGitReadOnlyCommand checks if a git command uses only read-safe subcommands
// with allowed flags. Returns true if the command should be auto-allowed.
// NOTE: git flags are case-sensitive (-M vs -m), so we only lowercase the
// subcommand name and global flags, preserving user-provided flag casing.
func bashROIsGitReadOnlyCommand(cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	if !strings.HasPrefix(strings.ToLower(trimmed), "git ") {
		return false
	}

	fields := strings.Fields(trimmed)
	gitIdx := -1
	for i, f := range fields {
		if strings.ToLower(f) == "git" {
			gitIdx = i
			break
		}
	}
	if gitIdx < 0 {
		return false
	}

	// Skip global flags (-C, -c, --git-dir, etc.) that take arguments.
	// Global flags are case-insensitive (git -C vs git -c), but subcommand
	// flags are case-sensitive, so we only lowercase global flags.
	subIdx := gitIdx + 1
	globalFlagsWithArgs := map[string]bool{
		"-c": true, "-C": true,
		"--git-dir": true, "--work-tree": true,
		"--namespace": true, "--super-prefix": true,
	}
	for subIdx < len(fields) {
		fld := fields[subIdx]
		if !strings.HasPrefix(fld, "-") {
			break
		}
		if strings.Contains(fld, "=") {
			subIdx++
			continue
		}
		if globalFlagsWithArgs[strings.ToLower(fld)] {
			subIdx += 2
			continue
		}
		subIdx++
	}
	if subIdx >= len(fields) {
		return false
	}

	// Subcommand name is case-insensitive: "diff", "log", etc.
	sub := strings.ToLower(fields[subIdx])
	key := "git " + sub
	// For two-level subcommands, check if next token is a sub2.
	// Only consider it a sub2 if it starts with a dash (--get, show, list)
	// to avoid treating positional args like "user.name" as sub2.
	if subIdx+1 < len(fields) {
		next := fields[subIdx+1]
		// Two-level subcommand: the second level must look like a git subcommand
		// or a well-known sub2 (--get, show, list, etc.), NOT a positional arg.
		// Rule: if the next token starts with "--", it could be a sub2 (e.g., --get).
		// Otherwise, only match if it's a known sub2 for the current subcommand.
		candidateKey2 := key + " " + strings.ToLower(next)
		if bashROGitCommands[candidateKey2] != nil {
			key = candidateKey2
			subIdx++
		}
	}

	config, ok := bashROGitCommands[key]
	if !ok {
		return false
	}

	remainingArgs := fields[subIdx+1:]
	return bashROValidateGitFlags(config, remainingArgs)
}
