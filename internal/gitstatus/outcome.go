package gitstatus

import "strings"

// The argv cannot tell these apart (a bare pull runs the same command for merge and rebase), so they are deduced from the output git already printed.
const (
	outcomeRebase          = "rebase"
	outcomeRebaseAutostash = "rebase+autostash"
	outcomeMerge           = "merge"
	outcomeFastForward     = "fast-forward"
	outcomeUpToDate        = "up-to-date"
	outcomeDiverged        = "diverged"
	outcomeConflict        = "conflict"
	outcomeNoUpstream      = "no-upstream"
	outcomePushed          = "pushed"
	outcomeRejected        = "rejected"
	outcomeFailed          = "failed"
)

// Reads output, not config: pull.rebase vs branch.<name>.rebase precedence shifts across git versions, so replicating it here would give a plausible wrong answer.
func Classify(args []string, out string, exitCode int) string {
	if len(args) == 0 {
		return ""
	}
	low := strings.ToLower(out)
	switch args[0] {
	case "pull", "fetch":
		return classifySync(low, exitCode)
	case "push":
		return classifyPush(low, exitCode)
	}
	return ""
}

func classifySync(low string, code int) string {
	if code != 0 {
		switch {
		// "conflict" is the shared signal: "could not apply <sha>" (rebase) and "Automatic merge failed" (merge).
		case strings.Contains(low, "could not apply"), strings.Contains(low, "conflict"):
			return outcomeConflict
		case strings.Contains(low, "not possible to fast-forward"),
			strings.Contains(low, "diverging branches"),
			strings.Contains(low, "divergent branches"):
			return outcomeDiverged
		case strings.Contains(low, "no tracking information"),
			strings.Contains(low, "no upstream"):
			return outcomeNoUpstream
		}
		return outcomeFailed
	}
	switch {
	// Autostash only shows up with rebase.autostash on, so "rebase+autostash" says more than plain "rebase".
	case strings.Contains(low, "successfully rebased"):
		if strings.Contains(low, "autostash") {
			return outcomeRebaseAutostash
		}
		return outcomeRebase
	case strings.Contains(low, "merge made by"):
		return outcomeMerge
	case strings.Contains(low, "fast-forward"):
		return outcomeFastForward
	case strings.Contains(low, "up to date"), strings.Contains(low, "up-to-date"):
		return outcomeUpToDate
	}
	return ""
}

func classifyPush(low string, code int) string {
	if code != 0 {
		if strings.Contains(low, "rejected") {
			return outcomeRejected
		}
		return outcomeFailed
	}
	if strings.Contains(low, "up-to-date") || strings.Contains(low, "up to date") {
		return outcomeUpToDate
	}
	return outcomePushed
}
