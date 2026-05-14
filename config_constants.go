package main

// Config constants ported from upstream: src/utils/configConstants.ts

// NotificationChannels lists the available notification channel options.
var NotificationChannels = []string{
	"auto",
	"iterm2",
	"iterm2_with_bell",
	"terminal_bell",
	"kitty",
	"ghostty",
	"notifications_disabled",
}

// EditorModes lists the valid editor modes.
var EditorModes = []string{"normal", "vim"}

// TeammateModes lists the valid teammate modes for spawning.
var TeammateModes = []string{"auto", "tmux", "in-process"}
