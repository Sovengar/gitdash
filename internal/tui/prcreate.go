// Creación del PR/MR: la ejecución del envío que el overlay recogió.
//
// NO es un handoff de terminal. gh y glab son no interactivos cuando todos los
// flags vienen dados (que es exactamente lo que garantiza forge.BuildCreateArgv:
// el cuerpo se emite siempre, y glab lleva su -y), así que no hay TTY que ceder
// ni editor que abrir: se captura la salida, como el comando `!`, y el usuario
// no sale de gitdash ni un segundo.
//
// La cadena es deliberadamente larga —remote, forge, argv, binario— y cada paso
// corta ANTES de ejecutar nada. Una creación de PR lanzada sin destino conocido
// (el `-R` de un repo adivinado) no falla visiblemente: crea el PR en el sitio
// equivocado. Por eso "no sé de dónde es" produce un toast que dice qué hacer y
// nada más.
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

// prStartMsg ordena lanzar la creación de lo que el overlay aceptó.
//
// Existe para que ACEPTAR el formulario y EJECUTARLO sean dos pasos y no uno.
// El overlay publica el envío en m.prPending y este mensaje lo consume: un test
// puede así observar el envío aceptado sin que ningún proceso haya salido, que
// es la mitad que un handoff de terminal (editor, lazygit) no tiene.
type prStartMsg struct{}

// prResultMsg entrega el desenlace de la creación. reject es el motivo por el
// que no se ejecutó NADA (sin remote, forge sin declarar, sin binario): en ese
// caso argv viene vacío y no hay entrada en el command log, porque no hubo
// proceso que registrar.
type prResultMsg struct {
	path   string
	argv   []string
	out    string
	err    error
	reject string
}

// prCreateCmd lanza la creación del PR/MR aceptado por el overlay. Consume el
// envío de m.prPending: a partir de aquí, o se ejecuta o se explica por qué no.
//
// Los pasos 1-4 (leer el remote, resolver el forge, armar el argv, comprobar el
// binario) van en la goroutine porque el primero es un `git remote get-url` y el
// último puede ser una CLI que tarda: en el hilo principal congelarían la TUI.
// El guard de "ya hay una acción en curso" y el consumo del envío NO: son
// estado del modelo, y se resuelven en el hilo principal como los de las demás
// acciones.
func (m *Model) prCreateCmd() tea.Cmd {
	sub := m.prPending
	if sub == nil {
		return nil
	}
	// Se consume antes de hacer nada: un envío que falló no se reintenta solo
	// porque un paso intermedio no llegara. Reintentar es abrir el formulario
	// otra vez.
	m.prPending = nil
	if cmd := m.busyActionCmd(sub.path); cmd != nil {
		return cmd
	}
	m.running[sub.path] = "pr"

	appCtx, events := m.ctx, m.events
	// Los mapas de forge se resuelven AQUÍ, no en la goroutine: salen de la
	// config del usuario, que solo se lee en el hilo principal.
	hosts, prefixes := m.cfg.ForgeHosts(), m.cfg.ForgePrefixes()
	// El nombre visible también: la goroutine no puede tocar m.projects.
	repo := m.nameOf(sub.path)
	go func() {
		// Cada corte sale por aquí: mismo camino, mismo mensaje al modelo, y el
		// handler puede tratar los cuatro igual (running libre, sin exec, sin
		// recollect).
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
		// Un forge sin puerta es un caso defensivo, no raro: la config avisa al
		// cargar de un proveedor que no soportamos, así que aquí solo se evita
		// ejecutar un argv vacío.
		if bin == "" || len(argv) == 0 {
			fail(fmt.Sprintf("%s: %s has no PR support in gitdash", repo, ref.Forge))
			return
		}
		if _, err := exec.LookPath(bin); err != nil {
			fail(fmt.Sprintf("%s: %s not installed", repo, bin))
			return
		}
		// El plazo (tool.DefaultTimeout()) lo aplica el Runner, no el contexto de
		// la app: uno cancela gitdash, el otro una CLI colgada.
		start := time.Now()
		out, runErr := tool.New(bin, forge.PromptEnv(ref)...).Run(appCtx, argv[1:]...)
		// A diferencia de los handoffs, aquí SÍ se mide: el proceso corre en
		// segundo plano con la salida capturada, así que su duración existe y
		// el command log tiene que guardarla.
		//
		// El argv viaja CRUDO: lo que se ejecutó, tal cual. El título y el cuerpo
		// los escribió una persona y son texto no confiable para la pintura,
		// pero el registro es la verdad de lo que corrió y lo sanea quien pinta
		// (sanitizeLogText, en el panel). Un registro "limpiado" sería un log
		// que puede mentir.
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

// prRemoteReject compone el aviso de un remote que no se pudo leer. El caso
// normal es que no haya `origin`, y ahí lo útil es QUÉ HACER (configurarlo), no
// el texto de git. Cuando git dice otra cosa (un repo roto, un binario que no
// está) el motivo sí se enseña entero: es lo único que hay.
func prRemoteReject(repo string, err error) string {
	if reason := prGitReason(err); reason != "" {
		return fmt.Sprintf("%s: cannot read origin — %s", repo, reason)
	}
	return fmt.Sprintf("no origin remote in %s — configure one first", repo)
}

// prGitReason saca el motivo real de un error de gitstatus (que viene con el
// argv delante) para que el toast lo enseñe sin el ruido del verbo. Vacío para
// el "no hay remote", que el prefijo del aviso ya dice.
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

// prNote compone el toast de un envío que SÍ se ejecutó: el éxito lleva la URL
// que imprimió la CLI (es lo que el usuario quiere copiar), y el fallo el motivo
// de la propia CLI, que es mucho más útil que el código de salida.
//
// El motivo sale de *tool.Error.Msg (la primera línea de su stderr) y no de su
// Error(), que repetiría el argv entero —con el título y el cuerpo dentro— en un
// aviso de tres líneas.
func prNote(repo string, msg prResultMsg) (toastLevel, string) {
	if msg.err != nil {
		return toastError, fmt.Sprintf("%s: PR failed — %s", repo, tool.FirstLine(prFailureReason(msg.err)))
	}
	if url := prURL(msg.out); url != "" {
		return toastSuccess, fmt.Sprintf("%s: PR created — %s", repo, url)
	}
	return toastSuccess, fmt.Sprintf("%s: PR created", repo)
}

// prFailureReason devuelve el motivo que la CLI imprimió por stderr, o el del
// proceso si no hubo nada (un binario que no arrancó, un plazo agotado).
// Runner lo garantiza lleno, así que el Toast nunca sale con el hueco vacío.
func prFailureReason(err error) string {
	var cerr *tool.Error
	if errors.As(err, &cerr) {
		return cerr.Msg
	}
	return err.Error()
}

// prURL busca la primera línea de la salida que sea una URL: gh y glab la
// imprimen al crear el PR, pero no siempre es la primera línea (gh puede dejar
// antes una línea de texto) y lo que el usuario quiere del aviso es el enlace.
func prURL(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			return line
		}
	}
	return ""
}
