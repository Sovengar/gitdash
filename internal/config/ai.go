package config

import "strings"

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
