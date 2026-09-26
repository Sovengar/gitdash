package gitstatus

import "strings"

// Clasificaciones de resultado que el command log muestra. Son las formas en
// que git puede reconciliar un pull/fetch o consumar un push; el argv no las
// distingue (con `pull` pelado, merge y rebase son el mismo comando), así que
// se deducen de la salida que git ya imprimió.
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

// Classify deduce qué hizo git realmente a partir de su salida combinada. Solo
// tiene sentido para los verbos cuya política se deduce del output (pull, fetch,
// push); el resto devuelve "" y el panel se queda con el código de salida.
//
// Se mira la salida y no la config a propósito: `git config pull.rebase` y
// `branch.<name>.rebase` tienen una precedencia que cambia entre versiones de
// git, así que replicarla en gitdash daría una respuesta plausible y
// equivocada. Lo que git HIZO sí está en su output, y con LC_ALL=C (forzado en
// gitEnv) los mensajes no se localizan.
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

// classifySync cubre pull y fetch: reconciliación de la rama con su upstream.
func classifySync(low string, code int) string {
	if code != 0 {
		switch {
		// "error: could not apply <sha>…" (rebase) y "Automatic merge
		// failed; fix conflicts…" (merge) comparten la palabra "conflict",
		// que es la señal común a los dos. Si además queda un rebase a
		// medias lo dice RebaseInProgress, no esta función.
		case strings.Contains(low, "could not apply"), strings.Contains(low, "conflict"):
			return outcomeConflict
		// Tres formas de lo mismo según qué flags y qué config se cruzó:
		// el fatal de --ff-only, su hint, y el fatal de "no sabría cómo
		// reconciliar" (que es lo que pasa sin pull.rebase en el gitconfig).
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
	// El autostash solo aparece si rebase.autostash está activo, así que
	// "rebase+autostash" dice más que "rebase" a secas.
	case strings.Contains(low, "successfully rebased"):
		if strings.Contains(low, "autostash") {
			return outcomeRebaseAutostash
		}
		return outcomeRebase
	case strings.Contains(low, "merge made by"):
		return outcomeMerge
	case strings.Contains(low, "fast-forward"):
		return outcomeFastForward
	// "Already up to date.", "Current branch X is up to date." y
	// "Everything up-to-date" (push) share forma con guion o sin él.
	case strings.Contains(low, "up to date"), strings.Contains(low, "up-to-date"):
		return outcomeUpToDate
	}
	return ""
}

// classifyPush cubre el consumo de commits locales al upstream.
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
