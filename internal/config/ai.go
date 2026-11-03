package config

import "strings"

// AIAgent routes an AI action to a known harness. The argv comes from the
// shared github.com/sovengar/agentbridge capability table, so gitdash holds no flags per
// agent.
type AIAgent struct {
	Harness string
	Model   string
	Effort  string
}

// AIAgentFor returns the harness route configured for an action.
func (c Config) AIAgentFor(action string) AIAgent {
	return c.AIAgents[action]
}

// ExpandAIVars resolves the {branch}/{behind}/{prompt} placeholders in one
// string. The prompt is one value, never re-scanned: the replacements are not
// applied to values they produce.
func ExpandAIVars(text string, prompt string, vars map[string]string) string {
	pairs := []string{"{prompt}", prompt}
	for k, v := range vars {
		pairs = append(pairs, "{"+k+"}", v)
	}
	return strings.NewReplacer(pairs...).Replace(text)
}

// Security boundary: the prompt becomes ONE argv element, never interpolated into an sh -c, and the {branch}/{behind} vars arrive resolved because config must not import gitstatus.
func BuildAIArgv(template, prompt string, vars map[string]string) []string {
	fields := strings.Fields(template)
	if len(fields) == 0 {
		return nil
	}
	pairs := []string{"{prompt}", prompt}
	for k, v := range vars {
		pairs = append(pairs, "{"+k+"}", v)
	}
	replacer := strings.NewReplacer(pairs...)

	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = replacer.Replace(f)
	}
	return out
}
