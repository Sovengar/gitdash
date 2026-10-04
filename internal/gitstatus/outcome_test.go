package gitstatus

import "testing"

// Copied verbatim from git with the LC_ALL=C gitdash forces, so the classifier can use exact strings; `git pull` output IS localized, so without LC_ALL=C a Spanish-locale user matches no signature and the panel prints "-".
func TestClassifyPullAndFetch(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		out  string
		code int
		want string
	}{
		{
			name: "rebase with autostash (what pp does with pull.rebase=true)",
			args: []string{"pull"},
			out: "Created autostash: 1b2c3d4\n" +
				"Rebasing (1/3)Rebasing (2/3)Rebasing (3/3)Applied autostash.\n" +
				"Successfully rebased and updated refs/heads/main.\n",
			want: "rebase+autostash",
		},
		{
			name: "rebase without autostash (clean tree)",
			args: []string{"pull"},
			out:  "Successfully rebased and updated refs/heads/main.\n",
			want: "rebase",
		},
		{
			name: "merge by default",
			args: []string{"pull"},
			out:  "Merge made by the 'ort' strategy.\n a.txt | 1 +\n",
			want: "merge",
		},
		{
			name: "fast-forward",
			args: []string{"pull"},
			out:  "Updating 8299325..13be697\nFast-forward\n zz | 1 +\n",
			want: "fast-forward",
		},
		{
			name: "nothing to integrate (the case where the reflog writes nothing)",
			args: []string{"pull"},
			out:  "Current branch main is up to date.\n",
			want: "up-to-date",
		},
		{
			name: "autostash created but nothing to integrate",
			args: []string{"pull"},
			out:  "Created autostash: 1b2c3d4\nCurrent branch main is up to date.\n",
			want: "up-to-date",
		},
		{
			name: "ff-only over diverged branches",
			args: []string{"pull", "--ff-only"},
			out: "hint: Diverging branches can't be fast-forwarded, you need to either:\n" +
				"fatal: Not possible to fast-forward, aborting.\n",
			code: 128,
			want: "diverged",
		},
		{
			name: "neither pull.rebase nor pull.default with diverged branches",
			args: []string{"pull"},
			out:  "fatal: You have divergent branches and need to specify how to reconcile them.\n",
			code: 128,
			want: "diverged",
		},
		{
			name: "conflict in a mid-rebase",
			args: []string{"pull", "--rebase"},
			out:  "Auto-merging a.txt\nCONFLICT (content): Merge conflict in a.txt\nerror: could not apply c6cdf09... # A\n",
			code: 1,
			want: "conflict",
		},
		{
			name: "conflict in a merge",
			args: []string{"pull"},
			out:  "Auto-merging a.txt\nCONFLICT (content): Merge conflict in a.txt\nAutomatic merge failed; fix conflicts and then commit the result.\n",
			code: 1,
			want: "conflict",
		},
		{
			name: "branch with no upstream",
			args: []string{"pull"},
			out:  "There is no tracking information for the current branch.\n",
			code: 1,
			want: "no-upstream",
		},
		{
			name: "generic failure (network, auth, disk)",
			args: []string{"pull"},
			out:  "fatal: Could not read from remote repository.\n",
			code: 128,
			want: "failed",
		},
		{
			name: "fetch up to date",
			args: []string{"fetch", "--prune"},
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.args, tc.out, tc.code); got != tc.want {
				t.Errorf("Classify(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestClassifyPush(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  string
		code int
		want string
	}{
		{
			name: "push consumed",
			out:  "To /tmp/origin.git\n   abc1234..def5678  main -> main\n",
			want: "pushed",
		},
		{
			name: "nothing to push",
			out:  "Everything up-to-date\n",
			want: "up-to-date",
		},
		{
			name: "rejected for not fast-forward",
			out:  " ! [rejected]        main -> main (fetch first)\nerror: failed to push some refs\n",
			code: 1,
			want: "rejected",
		},
		{
			name: "generic failure",
			out:  "fatal: unable to access 'https://…': Could not resolve host\n",
			code: 128,
			want: "failed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify([]string{"push"}, tc.out, tc.code); got != tc.want {
				t.Errorf("Classify(push) = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClassifyIgnoresVerbsWithoutPolicy(t *testing.T) {
	for _, args := range [][]string{
		{"worktree", "remove", "/tmp/wt"},
		{"status", "--porcelain=v2", "--branch"},
		{"rev-list", "--count", "HEAD..main"},
		{},
	} {
		if got := Classify(args, "Merge made by the 'ort' strategy.\n", 0); got != "" {
			t.Errorf("Classify(%v) = %q, want \"\"", args, got)
		}
	}
}

func TestClassifyIgnoresUppercase(t *testing.T) {
	if got := Classify([]string{"pull"}, "successfully REBASED and updated refs/heads/main.\n", 0); got != "rebase" {
		t.Errorf("Classify = %q, want %q", got, "rebase")
	}
}
