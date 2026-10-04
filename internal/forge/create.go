package forge

import "strings"

// Everything the CLI would otherwise ask travels here explicitly, which is what makes creation TTY-free and capturable as an exec instead of a terminal handoff.
type Params struct {
	Title  string
	Body   string
	Base   string
	Head   string
	Draft  bool
	Labels []string
}

// The "" is not a rare case: it is what stops the action before anything is executed.
func CreateBin(ref RepoRef) string {
	switch normalizeForge(ref.Forge) {
	case ForgeGitHub:
		return "gh"
	case ForgeGitLab:
		return "glab"
	default:
		return ""
	}
}

// The host travels in the environment, not the argv: `glab mr create` has no `--hostname` flag and GITLAB_HOST is its documented way to pick the instance (which api_base and token apply).
func PromptEnv(ref RepoRef) []string {
	switch normalizeForge(ref.Forge) {
	case ForgeGitHub:
		return []string{"GH_PROMPT_DISABLED=1"}
	case ForgeGitLab:
		if h := normalizeHost(ref.Host); h != "" {
			return []string{"GITLAB_HOST=" + h}
		}
		return nil
	default:
		return nil
	}
}

// A best-effort argv for an unknown forge would be worse than none (gh and glab share no meaningful flag), and -R always carries the resolved ref so the CLI cannot infer another repository from the remote.
func BuildCreateArgv(ref RepoRef, p Params) []string {
	switch normalizeForge(ref.Forge) {
	case ForgeGitHub:
		return ghCreateArgv(ref, p)
	case ForgeGitLab:
		return glabCreateArgv(ref, p)
	default:
		return nil
	}
}

// The trap: in gh -b is the BODY and -B is the BASE, told apart only by case, and swapping them fails silently with a PR against a branch that does not exist.
func ghCreateArgv(ref RepoRef, p Params) []string {
	argv := []string{"gh", "pr", "create", "-t", p.Title}
	// The body is emitted even when empty: that flag is what removes the prompt, and omitting it would make gh open an editor with no TTY to lend.
	argv = append(argv, "-b", p.Body)
	// Base and head only when given: their CLIs have sane defaults and an empty value there is an error, not an omission.
	if p.Base != "" {
		argv = append(argv, "-B", p.Base)
	}
	if p.Head != "" {
		argv = append(argv, "-H", p.Head)
	}
	if p.Draft {
		argv = append(argv, "-d")
	}
	// -l repeated instead of a comma list: a label containing a comma is legal on both forges and the list would split it in two.
	argv = append(argv, labelArgs(p.Labels)...)
	return append(argv, "-R", ghRepoArg(ref))
}

// The same trap reversed: in glab -b is the BASE and -d is the DESCRIPTION, so the flag names say the opposite of gh and the two maps sit side by side on purpose.
func glabCreateArgv(ref RepoRef, p Params) []string {
	argv := []string{"glab", "mr", "create", "-t", p.Title}
	argv = append(argv, "-d", p.Body)
	if p.Base != "" {
		argv = append(argv, "-b", p.Base)
	}
	if p.Head != "" {
		argv = append(argv, "-s", p.Head)
	}
	if p.Draft {
		// Long form on purpose: glab has no short flag for draft (--wip is a different flag for the same thing).
		argv = append(argv, "--draft")
	}
	argv = append(argv, labelArgs(p.Labels)...)
	// -y is glab's equivalent of gh's all-explicit: it skips the submit confirmation glab would ask for even with title, description and base given.
	if ref.Project != "" {
		argv = append(argv, "-y", "-R", ref.Project)
	} else {
		argv = append(argv, "-y")
	}
	return argv
}

// Empty labels are dropped: the CLI accepts `-l ""` and the API rejects it much later without saying which label it was.
func labelArgs(labels []string) []string {
	var out []string
	for _, l := range labels {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, "-l", l)
		}
	}
	return out
}

// The host is spelled out only when it is not github.com, because that is what tells gh not to read the path as belonging to the public site.
func ghRepoArg(ref RepoRef) string {
	host := normalizeHost(ref.Host)
	if host != "" && host != "github.com" {
		return host + "/" + strings.Trim(ref.Project, "/")
	}
	return strings.Trim(ref.Project, "/")
}

// Lowercased like the host: the forge name comes from a hand-written config map, and a "GitHub" that missed the constant would leave the action with no door.
func normalizeForge(forge string) string {
	return strings.ToLower(strings.TrimSpace(forge))
}
