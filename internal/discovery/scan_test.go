package discovery

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitdash/internal/config"
	"gitdash/internal/testutil"
)

func cfgRoots(roots ...string) config.Config {
	cfg := config.Defaults()
	cfg.Roots = roots
	return cfg
}

func TestDetectByMarker(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "projects", "api")
	testutil.Init(t, proj)
	testutil.Marker(t, proj, "", "", "", false)

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(projects))
	}
	p := projects[0]
	if p.Path != proj || p.Name != "api" || !p.HasRepo || p.IsWorktree {
		t.Errorf("p = %+v", p)
	}
}

func TestPruneHiddenAndExcluded(t *testing.T) {
	root := t.TempDir()
	hidden := filepath.Join(root, ".hidden", "proj")
	excluded := filepath.Join(root, "api", "node_modules", "dep")
	for _, dir := range []string{hidden, excluded} {
		testutil.Init(t, dir)
		testutil.Marker(t, dir, "", "", "", false)
	}

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Errorf("projects = %d, want 0 (poda)", len(projects))
	}
}

func TestUnlimitedDepth(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "a", "b", "c", "d", "proj")
	testutil.Init(t, proj)
	testutil.Marker(t, proj, "", "", "", false)

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Name != "proj" {
		t.Errorf("projects = %+v", projects)
	}
}

func TestNestedValid(t *testing.T) {
	root := t.TempDir()
	mono := filepath.Join(root, "mono")
	sub := filepath.Join(mono, "sub")
	for _, dir := range []string{mono, sub} {
		testutil.Init(t, dir)
		testutil.Marker(t, dir, "", "", "", false)
	}

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("projects = %d, want 2 (mono + sub)", len(projects))
	}
}

// Scan entrega los proyectos ordenados por ruta, no en el orden del walk (que
// depende del sistema de ficheros). Sin esto, invertir el comparador no lo
// detecta nadie: la TUI vería los repos reordenados entre escaneos.
func TestScanOrdenaPorRuta(t *testing.T) {
	root := t.TempDir()
	// Se crean en orden inverso al que deben salir: el walk los devuelve en
	// orden de lectura del directorio, no en orden alfabético.
	creados := []string{"zeta", "alfa", "middle"}
	for _, name := range creados {
		dir := filepath.Join(root, name)
		testutil.Init(t, dir)
		testutil.Marker(t, dir, "", "", "", false)
	}

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 3 {
		t.Fatalf("projects = %d, want 3", len(projects))
	}
	want := []string{
		filepath.Join(root, "alfa"),
		filepath.Join(root, "middle"),
		filepath.Join(root, "zeta"),
	}
	for i, w := range want {
		if projects[i].Path != w {
			t.Errorf("projects[%d] = %q, want %q (orden por ruta)", i, projects[i].Path, w)
		}
	}
}

// Con varios roots el orden global sigue siendo por ruta, no "root a root": un
// root puede intercalar sus proyectos entre los del otro. Los roots se crean
// bajo un padre común y en orden invertido a propósito, para que ese entrecruzado
// sea observable: con el orden por root, "a-root" saldría al final y el test
// fallaría.
func TestScanOrdenaEntreRoots(t *testing.T) {
	base := t.TempDir()
	// "a-root" se escanea segundo pero ordena antes: sin el orden global por
	// ruta, sus proyectos saldrían al final.
	segundo := filepath.Join(base, "a-root")
	primero := filepath.Join(base, "z-root")
	for _, dir := range []string{
		filepath.Join(primero, "b"),
		filepath.Join(primero, "d"),
		filepath.Join(segundo, "a"),
		filepath.Join(segundo, "c"),
	} {
		testutil.Init(t, dir)
		testutil.Marker(t, dir, "", "", "", false)
	}

	projects, err := Scan(cfgRoots(primero, segundo))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 4 {
		t.Fatalf("projects = %d, want 4", len(projects))
	}
	want := []string{
		filepath.Join(segundo, "a"),
		filepath.Join(segundo, "c"),
		filepath.Join(primero, "b"),
		filepath.Join(primero, "d"),
	}
	for i, w := range want {
		if projects[i].Path != w {
			t.Errorf("projects[%d] = %q, want %q", i, projects[i].Path, w)
		}
	}
}

func TestWorktree(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "main-repo")
	testutil.Init(t, main)
	testutil.Marker(t, main, "", "", "", false)
	testutil.CommitFiles(t, main, map[string]string{"a.txt": "a"}, "init")

	wt := filepath.Join(root, "wt-proj")
	testutil.MakeWorktree(t, main, wt, "wt-branch")
	testutil.Marker(t, wt, "", "", "", false)

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("projects = %d, want 2", len(projects))
	}
	var found bool
	for _, p := range projects {
		if p.Path == wt {
			found = true
			if !p.IsWorktree || !p.HasRepo {
				t.Errorf("worktree mal clasificado: %+v", p)
			}
		}
	}
	if !found {
		t.Error("worktree no descubierto")
	}
}

func TestMarkerWithoutRepo(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "plain")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Marker(t, proj, "", "", "", false)

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].HasRepo {
		t.Errorf("projects = %+v, want 1 sin repo", projects)
	}
}

func TestMarkerMetadata(t *testing.T) {
	root := t.TempDir()
	withMeta := filepath.Join(root, "dirname")
	empty := filepath.Join(root, "emptymarker")
	bad := filepath.Join(root, "badmarker")
	for _, dir := range []string{withMeta, empty, bad} {
		testutil.Init(t, dir)
	}
	testutil.Marker(t, withMeta, "api", "vsocial", "", false)
	testutil.Marker(t, empty, "", "", "", false)
	testutil.Marker(t, bad, "", "", "", true)

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 3 {
		t.Fatalf("projects = %d, want 3", len(projects))
	}
	byPath := map[string]Project{}
	for _, p := range projects {
		byPath[p.Name] = p
	}
	if p := byPath["api"]; p.PrimaryGroup != "vsocial" {
		t.Errorf("name/primary = %q/%q", p.Name, p.PrimaryGroup)
	}
	if p := byPath["emptymarker"]; p.PrimaryGroup != "" {
		t.Errorf("primary = %q, want vacío", p.PrimaryGroup)
	}
	if p := byPath["badmarker"]; p.MarkerErr == "" {
		t.Error("marcador malformado sin error visible")
	}
}

// Primary_group/secondary_group del marcador; la clave
// vieja group ya no agrupa; secondary sin primary se ignora.
func TestMarkerGroups(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	oldKey := filepath.Join(root, "oldkey")
	secOnly := filepath.Join(root, "seconly")
	for _, dir := range []string{nested, oldKey, secOnly} {
		testutil.Init(t, dir)
	}
	testutil.Marker(t, nested, "api", "vsocial", "backend", false)
	if err := os.WriteFile(filepath.Join(oldKey, ".gitdash.toml"), []byte("group = \"backend\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Marker(t, secOnly, "solo", "", "infra", false)

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]Project{}
	for _, p := range projects {
		byPath[p.Name] = p
	}
	if p := byPath["api"]; p.PrimaryGroup != "vsocial" || p.SecondaryGroup != "backend" {
		t.Errorf("primary/secondary = %q/%q", p.PrimaryGroup, p.SecondaryGroup)
	}
	if p := byPath["oldkey"]; p.PrimaryGroup != "" || p.SecondaryGroup != "" {
		t.Errorf("group viejo agrupó: %q/%q", p.PrimaryGroup, p.SecondaryGroup)
	}
	if p := byPath["solo"]; p.PrimaryGroup != "" || p.SecondaryGroup != "" {
		t.Errorf("secondary sin primary: %q/%q", p.PrimaryGroup, p.SecondaryGroup)
	}
}

func TestIlegibleRootNoAborta(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "ok")
	testutil.Init(t, proj)
	testutil.Marker(t, proj, "", "", "", false)

	_, err := Scan(cfgRoots(root, filepath.Join(root, "fantasma")))
	if err == nil {
		t.Error("se esperaba error agregado por root ilegible")
	}
}

// Scan es tolerante a roots malos: los reporta como error agregado y sigue con
// los demas. Un root que no existe y un root que es un FICHERO (no un
// directorio) tienen que acabar los dos en el mismo saco, y ademas con el mismo
// texto de aviso, porque para el usuario son la misma cosa: "esta ruta de la
// config no se puede recorrer".
func TestScanToleraRootsInutiles(t *testing.T) {
	root := t.TempDir()
	bueno := filepath.Join(root, "proyecto")
	testutil.Init(t, bueno)
	testutil.Marker(t, bueno, "", "", "", false)

	// Un fichero donde deberia haber un directorio.
	noDir := filepath.Join(root, "un-fichero")
	if err := os.WriteFile(noDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := cfgRoots(root, filepath.Join(root, "no-existe"), noDir)
	projects, err := Scan(cfg)

	// El proyecto bueno se sigue descubriendo: el fallo de los otros no aborta.
	if len(projects) != 1 || projects[0].Path != bueno {
		t.Errorf("projects = %+v, want solo %s (un root malo no aborta el resto)", projects, bueno)
	}
	if err == nil {
		t.Fatal("Scan = nil error, want aviso de los roots ilegibles")
	}
	// Los dos malos se nombran, para que el usuario sepa cual quitar de la config.
	for _, quiere := range []string{"no-existe", "un-fichero"} {
		if !strings.Contains(err.Error(), quiere) {
			t.Errorf("error = %q, want que nombre %q", err, quiere)
		}
	}
	// Y el error es AGREGADO: los dos en el mismo error, no uno y el otro fuera.
	if n := strings.Count(err.Error(), "root ilegible"); n != 2 {
		t.Errorf("el error menciona %d roots ilegibles, want 2 (los que no son directorios)", n)
	}
}

// classifyGit distingue repo (dir), worktree (fichero gitdir:) y nada. El caso
// del `.git` INEXISTENTE es el de un marcador commiteado en un repo de pruebas
// que todavia no se ha inicializado: no es un error, es "sin repo", y asi lo
// tiene que decir el discoverer sin que Scan lo escupe.
func TestClassifyGitSinGit(t *testing.T) {
	dir := t.TempDir()
	if k, main := classifyGit(filepath.Join(dir, ".git")); k != gitNone || main != "" {
		t.Errorf("classifyGit(inexistente) = %v/%q, want gitNone/\"\"", k, main)
	}

	// Un .git que existe pero es un DIRECTORIO es un repo normal.
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if k, main := classifyGit(filepath.Join(repo, ".git")); k != gitDir || main != "" {
		t.Errorf("classifyGit(dir) = %v/%q, want gitDir/%q", k, main, "")
	}

	// Un .git que es un FICHERO pero no dice "gitdir:" no es un worktree
	// registrable: se descarta en vez de devolver un repo principal inventado.
	raro := filepath.Join(dir, "raro")
	if err := os.WriteFile(raro, []byte("no soy un gitdir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if k, main := classifyGit(raro); k != gitNone || main != "" {
		t.Errorf("classifyGit(fichero sin gitdir:) = %v/%q, want gitNone/%q", k, main, "")
	}
}

// Un worktree de verdad: `.git` es un fichero que apunta a
// <main>/.git/worktrees/<nombre>, y el repo principal son dos niveles arriba.
func TestClassifyGitWorktree(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "main-repo")
	wt := filepath.Join(dir, "feature")
	gitdir := filepath.Join(main, ".git", "worktrees", "feature")
	if err := os.MkdirAll(filepath.Join(wt), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(gitdir, 0o755); err != nil {
		t.Fatal(err)
	}
	pointer := filepath.Join(wt, ".git")
	if err := os.WriteFile(pointer, []byte("gitdir: "+gitdir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	k, repoPrincipal := classifyGit(pointer)
	if k != gitFile {
		t.Fatalf("classifyGit = %v, want gitFile (es un worktree)", k)
	}
	// El puntero acaba en <main>/.git/worktrees/<nombre>, y el repo principal es
	// dos niveles arriba de ahi, es decir el propio <main>.
	if want := main; repoPrincipal != want {
		t.Errorf("repo principal = %q, want %q", repoPrincipal, want)
	}
}

// El marcador de un worktree sintetico puede no existir (por ejemplo, un test
// que construye la forma sin el marcador committed). MarkerPrompt lo trata como
// vacio y no como error: la accion AI sin prompt es un "no hay nada que
// Mandar", no un fallo.
func TestMarkerPromptSinMarcadorNoEsError(t *testing.T) {
	dir := t.TempDir()
	p, err := MarkerPrompt(dir, ".gitdash.toml", "pull")
	if err != nil {
		t.Errorf("MarkerPrompt sin marcador = %v, want nil (sin prompt no hay error)", err)
	}
	if p != "" {
		t.Errorf("prompt = %q, want vacio", p)
	}
}

// Un directorio sin permiso de lectura es el caso real de la rama de error del
// walk: el usuario tiene un repo dentro de algo que no puede leer y Scan tiene
// que SALTARSELO, no abortar el escaneo entero. Abortar seria perder todos los
// repos que sí se ven por un directorio ajeno.
func TestScanSaltaDirectorioIlegible(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("como root los permisos de directorio no impiden la lectura")
	}
	root := t.TempDir()
	bueno := filepath.Join(root, "proyecto")
	testutil.Init(t, bueno)
	testutil.Marker(t, bueno, "", "", "", false)

	// Un directorio sin permiso, dentro del root pero sin marcador.
	bloqueado := filepath.Join(root, "sin-acceso")
	if err := os.MkdirAll(bloqueado, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(bloqueado, 0o755) })

	projects, err := Scan(cfgRoots(root))
	if len(projects) != 1 || projects[0].Path != bueno {
		t.Errorf("projects = %+v, want solo %s (un dir ilegible no aborta)", projects, bueno)
	}
	if err != nil {
		t.Errorf("Scan = %v, want nil (el dir ilegible se salta en silencio)", err)
	}
}

// MarkerPrompt con un marcador MALFORMADO sí es error, a diferencia del
// inexistente. La diferencia importa: sin prompt la acción AI no se lanza (un
// aviso), pero con un marcador corrupto el usuario tiene un `.gitdash.toml`
// roto que debe arreglar, y avisar de eso es mas util que decir "no hay prompt".
func TestMarkerPromptMalformadoEsError(t *testing.T) {
	dir := t.TempDir()
	writeMarker(t, dir, "name = [roto\n")
	p, err := MarkerPrompt(dir, ".gitdash.toml", "pull")
	if err == nil {
		t.Fatal("MarkerPrompt con marcador malformado = nil, want error (el fichero esta roto)")
	}
	if !strings.Contains(err.Error(), "marker") {
		t.Errorf("error = %q, want que nombre el marcador", err)
	}
	if p != "" {
		t.Errorf("prompt = %q con error, want vacio", p)
	}
}

// classifyGit con un `.git` que existe pero no se puede LEER (es un fichero sin
// permiso) no es un worktree: no hay gitdir que seguir, asi que se descarta. La
// comprobacion es de que no devuelve un repo principal inventado.
func TestClassifyGitConFicheroIlegible(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("como root los permisos de fichero no impiden la lectura")
	}
	dir := t.TempDir()
	git := filepath.Join(dir, ".git")
	if err := os.WriteFile(git, []byte("gitdir: /no/se/puede/leer\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(git, 0o644) })

	if k, main := classifyGit(git); k != gitNone || main != "" {
		t.Errorf("classifyGit(ilegible) = %v/%q, want gitNone/%q", k, main, "")
	}
}

// Un marcador que NO es un fichero (es un directorio) es un error de lectura, no
// una ausencia: por eso no cae en el caso de "sin marcador, sin prompt". El
// aviso importa porque hay algo que arreglar y no es "no hay prompt".
//
// Se dispara con un directorio en vez de con permisos a proposito: un 0o000 lo
// lee root, asi que un test de permisos diria una cosa en local y otra en CI,
// y este no depende de quien corre.
func TestMarkerPromptConMarcadorNoLegibleEsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gitdash.toml"), 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := MarkerPrompt(dir, ".gitdash.toml", "pull")
	if err == nil {
		t.Fatal("MarkerPrompt con un marcador que es un directorio = nil, want error")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want EISDIR y no ENOENT: un directorio NO es un marcador ausente", err)
	}
	if p != "" {
		t.Errorf("prompt = %q con error, want vacio", p)
	}
}

// Un root RELATIVO solo se puede resolver si el proceso tiene cwd, y `Abs`
// falla sin él. No es teórico: `filepath.Abs` llama a `os.Getwd`, que da ENOENT
// cuando el directorio de trabajo ya no existe (un repo movido o un `cd` a un
// tmpdir borrado por otro proceso).
//
// El aviso importa: sin él, un root relativo con el cwd roto devuelve cero
// proyectos y SIN error, que se lee como "no tengo repos" en vez de como "no
// puedo ni mirar dónde estoy".
func TestScanConCwdBorradoReportaElRoot(t *testing.T) {
	roto := t.TempDir()
	t.Chdir(roto)
	if err := os.RemoveAll(roto); err != nil {
		t.Fatal(err)
	}

	projects, err := Scan(cfgRoots("."))
	if err == nil {
		t.Fatalf("Scan con el cwd borrado = nil, want error (0 proyectos y sin aviso parece un root vacío): %+v", projects)
	}
	if len(projects) != 0 {
		t.Errorf("projects = %+v, want ninguno", projects)
	}
	if !strings.Contains(err.Error(), ".") {
		t.Errorf("error = %q, want que nombre el root que no pudo resolver", err)
	}
}

// `parseMarker` con un marcador que NO se puede leer. No hace falta un 0o000
// (que root lee): un DIRECTORIO con el nombre del marcador da EISDIR, y el
// error tiene que PROPAGARSE.
//
// `Scan` nunca llama aquí con algo ilegible —`hasMarker` exige que sea un
// fichero— así que esta es la única forma de que el error llegue al modelo: por
// eso se prueba la unidad, no el scan. El contrato que importa es que
// `inspect` lo mete en `MarkerErr`, que es lo que la TUI enseña.
func TestParseMarkerIlegiblePropagaElError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gitdash.toml"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := parseMarker(filepath.Join(dir, ".gitdash.toml"))
	if err == nil {
		t.Fatal("parseMarker = nil, want el error de lectura")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want EISDIR y no ENOENT", err)
	}
	// Y el modelo lo muestra en vez de tragárselo: un marcador ilegible es algo
	// que el usuario tiene que arreglar, no un proyecto sin metadatos.
	p := inspect(dir, ".gitdash.toml")
	if p.MarkerErr == "" {
		t.Errorf("MarkerErr vacio, want el error de lectura: %+v", p)
	}
	if p.Name != filepath.Base(dir) {
		t.Errorf("Name = %q, want el nombre del directorio (el marcador no dio ninguno)", p.Name)
	}
}
