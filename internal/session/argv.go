package session

import "strings"

var (
	passthroughFlags = map[string]struct{}{"p": {}, "print": {}, "prompt": {}, "h": {}, "help": {}, "version": {}}
	subcommands      = map[string]struct{}{"acp": {}, "agent": {}, "agents": {}, "changelog": {}, "help": {}, "install": {}, "mcp": {}, "mic-serve": {}, "models": {}, "plugin": {}, "plugins": {}, "remote-control": {}, "update": {}}
	sessionFlags     = map[string]struct{}{"c": {}, "continue": {}, "conversation": {}, "i": {}, "prompt-interactive": {}, "new-project": {}}
	valuedFlags      = map[string]struct{}{"conversation": {}, "i": {}, "prompt-interactive": {}}
)

// ShouldSupervise only allows a long-lived interactive invocation. Ambiguity
// resolves to false so non-session commands retain official agy behavior.
func ShouldSupervise(args []string, stdinTTY, stdoutTTY bool) bool {
	if !stdinTTY || !stdoutTTY {
		return false
	}
	for _, token := range args {
		name, isFlag := flagName(token)
		if isFlag {
			if _, ok := passthroughFlags[name]; ok {
				return false
			}
		} else if _, ok := subcommands[token]; ok {
			return false
		}
	}
	return true
}

// ResumeArgs removes flags that select/create the old conversation and adds
// the exact conversation ID plus the continuation prompt. A pinned model
// replaces any model argument in the original invocation.
func ResumeArgs(args []string, conversationID, prompt, model string) []string {
	dropModel := model != ""
	kept := make([]string, 0, len(args)+4)
	skipValue := false
	for _, token := range args {
		if skipValue {
			skipValue = false
			continue
		}
		name, isFlag := flagName(token)
		if isFlag {
			_, dropSession := sessionFlags[name]
			if dropSession || (dropModel && name == "model") {
				_, takesValue := valuedFlags[name]
				if dropModel && name == "model" {
					takesValue = true
				}
				skipValue = takesValue && !strings.Contains(token, "=")
				continue
			}
		}
		kept = append(kept, token)
	}
	if dropModel {
		kept = append(kept, "--model", model)
	}
	kept = append(kept, "--conversation="+conversationID, "-i", prompt)
	return kept
}

func flagName(token string) (string, bool) {
	if token == "" || token == "-" || token == "--" || !strings.HasPrefix(token, "-") {
		return "", false
	}
	name := strings.TrimLeft(token, "-")
	if name == "" {
		return "", false
	}
	if before, _, found := strings.Cut(name, "="); found {
		name = before
	}
	return name, true
}
