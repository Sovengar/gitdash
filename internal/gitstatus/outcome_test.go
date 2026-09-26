package gitstatus

import "testing"

// Las salidas de estos casos están copiadas literalmente de git (con LC_ALL=C,
// que es lo que gitdash fuerza): no son inventadas, y por eso el clasificador
// puede quedarse con cadenas exactas en vez de heurísticas.
//
// Verificado también que los mensajes del reflog y del porcelain NO se
// localizan (salen en inglés con LANG=es_ES), pero las salidas de `git pull` sí
// se traducen — de ahí el LC_ALL=C forzado en gitEnv: sin él, un usuario en
// locale español no matchearía ninguna firma y el panel diría "-" en vez de
// "rebase".
func TestClassifyPullYFetch(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		out  string
		code int
		want string
	}{
		{
			name: "rebase con autostash (lo que hace pp con pull.rebase=true)",
			args: []string{"pull"},
			out: "Created autostash: 1b2c3d4\n" +
				"Rebasing (1/3)Rebasing (2/3)Rebasing (3/3)Applied autostash.\n" +
				"Successfully rebased and updated refs/heads/main.\n",
			want: "rebase+autostash",
		},
		{
			name: "rebase sin autostash (árbol limpio)",
			args: []string{"pull"},
			out:  "Successfully rebased and updated refs/heads/main.\n",
			want: "rebase",
		},
		{
			name: "merge por defecto",
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
			name: "nada que integrar (el caso donde el reflog no escribe nada)",
			args: []string{"pull"},
			out:  "Current branch main is up to date.\n",
			want: "up-to-date",
		},
		{
			name: "autostash creado pero sin nada que integrar",
			args: []string{"pull"},
			out:  "Created autostash: 1b2c3d4\nCurrent branch main is up to date.\n",
			want: "up-to-date",
		},
		{
			name: "ff-only sobre ramas divergentes",
			args: []string{"pull", "--ff-only"},
			out: "hint: Diverging branches can't be fast-forwarded, you need to either:\n" +
				"fatal: Not possible to fast-forward, aborting.\n",
			code: 128,
			want: "diverged",
		},
		{
			name: "sin pull.rebase ni pull.default con ramas divergentes",
			args: []string{"pull"},
			out:  "fatal: You have divergent branches and need to specify how to reconcile them.\n",
			code: 128,
			want: "diverged",
		},
		{
			name: "conflicto en un rebase a medias",
			args: []string{"pull", "--rebase"},
			out:  "Auto-merging a.txt\nCONFLICT (content): Merge conflict in a.txt\nerror: could not apply c6cdf09... # A\n",
			code: 1,
			want: "conflict",
		},
		{
			name: "conflicto en un merge",
			args: []string{"pull"},
			out:  "Auto-merging a.txt\nCONFLICT (content): Merge conflict in a.txt\nAutomatic merge failed; fix conflicts and then commit the result.\n",
			code: 1,
			want: "conflict",
		},
		{
			name: "rama sin upstream",
			args: []string{"pull"},
			out:  "There is no tracking information for the current branch.\n",
			code: 1,
			want: "no-upstream",
		},
		{
			name: "fallo genérico (red, auth, disco)",
			args: []string{"pull"},
			out:  "fatal: Could not read from remote repository.\n",
			code: 128,
			want: "failed",
		},
		{
			name: "fetch al día",
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
			name: "nada que publicar",
			out:  "Everything up-to-date\n",
			want: "up-to-date",
		},
		{
			name: "rechazado por no fast-forward",
			out:  " ! [rejected]        main -> main (fetch first)\nerror: failed to push some refs\n",
			code: 1,
			want: "rejected",
		},
		{
			name: "fallo genérico",
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

// Solo los verbos cuya política se deduce de la salida se clasifican. El resto
// (worktree, status, log…) devuelve "" y el panel se queda con el código de
// salida: inventar un "resultado" para un `git worktree remove` sería mentir.
func TestClassifyIgnoraVerbosSinPolitica(t *testing.T) {
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

// La comparación es sobre el texto en minúsculas: git capitaliza según el
// idioma y el caso ("Fast-forward" vs "fast-forward").
func TestClassifyIgnoraMayusculas(t *testing.T) {
	if got := Classify([]string{"pull"}, "successfully REBASED and updated refs/heads/main.\n", 0); got != "rebase" {
		t.Errorf("Classify = %q, want %q", got, "rebase")
	}
}
