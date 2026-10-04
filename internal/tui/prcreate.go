// NOT a terminal handoff (gh and glab are non-interactive once every flag is given), so the output is captured; the chain is long and every step cuts BEFORE executing because a PR created with a guessed `-R` does not fail, it lands on the wrong site.
package tui

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"gitdash/internal/cmdlog"
	"gitdash/internal/forge"
	"gitdash/internal/forge/tool"
	"gitdash/internal/gitstatus"
)

// It exists so ACCEPTING the form and EXECUTING it are two steps: a test can observe the accepted submission with no process having left, which is the half a terminal handoff does not have.
type prStartMsg struct{}

type prResultMsg struct {
	path   string
	argv   []string
	out    string
	err    error
	reject string
}

// Steps 1-4 (remote, forge, argv, binary) go in the goroutine because the first is a `git remote get-url` and the last a slow CLI; the busy guard and consuming the submission do not, since those are model state.
func (m *Model) prCreateCmd() tea.Cmd {
	sub := m.prPending
	if sub == nil {
		return nil
	}
	// Consumed before doing anything: a submission that failed must not retry itself because an intermediate step did not arrive; retrying means opening the form again.
	m.prPending = nil
	if cmd := m.busyActionCmd(sub.path); cmd != nil {
		return cmd
	}
	m.running[sub.path] = "pr"

	appCtx, events := m.ctx, m.events
	// The forge maps are resolved HERE and not in the goroutine: they come from the user's config, which is only read on the main goroutine.
	hosts, prefixes := m.cfg.ForgeHosts(), m.cfg.ForgePrefixes()
	// The visible name too: the goroutine must not touch m.projects.
	repo := m.nameOf(sub.path)
	go func() {
		fail := func(reason string) {
			sendEvent(appCtx, events, prResultMsg{path: sub.path, reject: reason})
		}
		raw, err := gitstatus.RemoteURL(appCtx, sub.path)
		if err != nil {
			fail(prRemoteReject(repo, err))
			return
		}
		ref, ok := forge.ParseRemoteURL(raw, hosts, prefixes)
		if !ok {
			fail(fmt.Sprintf("%s: no forge for this remote — declare the host in [forge.github] or [forge.gitlab]", repo))
			return
		}
		argv, bin := forge.BuildCreateArgv(ref, sub.params), forge.CreateBin(ref)
		// A forge with no door is a defensive case, not a rare one: the config already warns about an unsupported provider on load, so this only avoids running an empty argv.
		if bin == "" || len(argv) == 0 {
			fail(fmt.Sprintf("%s: %s has no PR support in gitdash", repo, ref.Forge))
			return
		}
		if _, err := exec.LookPath(bin); err != nil {
			fail(fmt.Sprintf("%s: %s not installed", repo, bin))
			return
		}
		// The deadline (tool.DefaultTimeout()) is applied by the Runner and not by the app context: one cancels gitdash, the other a hung CLI.
		start := time.Now()
		out, runErr := tool.New(bin, forge.PromptEnv(ref)...).Run(appCtx, argv[1:]...)
		// Unlike the handoffs the duration IS measured (the process runs in background with its output captured), and the argv travels RAW: it is what ran, and the painter sanitizes it, since a "cleaned" record would be a log that can lie.
		cmdlog.RecordExec(cmdlog.Entry{
			Repo:   repo,
			Dir:    sub.path,
			Class:  cmdlog.ClassAction,
			Action: "pr",
			Argv:   argv,
			Exit:   tool.ExitCode(runErr),
			Dur:    time.Since(start),
		})
		sendEvent(appCtx, events, prResultMsg{path: sub.path, argv: argv, out: out, err: runErr})
	}()
	return nil
}

// The normal case is a missing `origin`, and what helps there is WHAT TO DO (configure it), not git's text; when git says something else (a broken repo, a missing binary) the reason is shown whole, since it is all there is.
func prRemoteReject(repo string, err error) string {
	if reason := prGitReason(err); reason != "" {
		return fmt.Sprintf("%s: cannot read origin — %s", repo, reason)
	}
	return fmt.Sprintf("no origin remote in %s — configure one first", repo)
}

func prGitReason(err error) string {
	reason := tool.FirstLine(err.Error())
	if strings.Contains(reason, "No such remote") {
		return ""
	}
	if strings.HasPrefix(reason, "git ") {
		if i := strings.Index(reason, "]: "); i >= 0 {
			reason = reason[i+len("]: "):]
		}
	}
	return reason
}

// The reason comes from *tool.Error.Msg (its stderr's first line) and not from its Error(), which would repeat the whole argv (title and body included) inside a three-line warning.
func prNote(repo string, msg prResultMsg) (toastLevel, string) {
	if msg.err != nil {
		return toastError, fmt.Sprintf("%s: PR failed — %s", repo, tool.FirstLine(prFailureReason(msg.err)))
	}
	if url := prURL(msg.out); url != "" {
		return toastSuccess, fmt.Sprintf("%s: PR created — %s", repo, url)
	}
	return toastSuccess, fmt.Sprintf("%s: PR created", repo)
}

func prFailureReason(err error) string {
	var cerr *tool.Error
	if errors.As(err, &cerr) {
		return cerr.Msg
	}
	return err.Error()
}

// gh may print a text line before the URL, and the warning is about the link.
func prURL(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			return line
		}
	}
	return ""
}
