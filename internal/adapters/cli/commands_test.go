package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nexus/internal/catalog"
	platformdb "nexus/internal/platform/db"
	"nexus/internal/tracking"
)

// Adaptadores mínimos que replican el cableado de cmd/nexus para probar la CLI de punta a punta.
type testEntries struct{ tracker *tracking.Tracker }

func (e testEntries) Relink(fromID, toID int64, toName string) error {
	return e.tracker.Relink(fromID, toID, toName)
}
func (e testEntries) RenameProject(id int64, name string) error {
	return e.tracker.RenameProject(id, name)
}

type testProjects struct{ repo *catalog.SQLite }

func (p testProjects) EnsureProject(name string) (int64, error) {
	project, err := p.repo.EnsureProject(name)
	if errors.Is(err, catalog.ErrAmbiguousProject) {
		return 0, errors.Join(tracking.ErrAmbiguousProject, catalog.ErrAmbiguousProject)
	}
	return project.ID, err
}

func (p testProjects) ProjectName(id int64) (string, error) {
	proj, err := p.repo.Project(id)
	if err != nil {
		return "", err
	}
	return proj.Name, nil
}

type app struct {
	tracker *tracking.Tracker
	catalog *catalog.Service
	now     *time.Time
}

func newApp(t *testing.T) app {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nexus.db")
	db, err := platformdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	steps := tracking.Migrations()
	migrations := append([]platformdb.Migration{steps[0]}, catalog.Migrations()...)
	migrations = append(migrations, steps[1:]...)
	if _, _, err := platformdb.Migrate(db, path, migrations, time.Now); err != nil {
		t.Fatal(err)
	}
	current := time.Date(2025, 3, 4, 12, 0, 0, 0, time.Local)
	now := func() time.Time { return current }
	repo := catalog.NewSQLite(db)
	tracker := tracking.NewTracker(tracking.NewSQLite(db), now, tracking.WithProjects(testProjects{repo}))
	return app{tracker, catalog.NewService(repo, testEntries{tracker}, now), &current}
}

func (a app) run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out, stderr bytes.Buffer
	clk := time.Now
	if a.now != nil {
		clk = func() time.Time { return *a.now }
	}
	err := RunWithOptions(args, a.tracker, Options{Catalog: a.catalog, Now: clk}, &out, &stderr)
	return out.String(), err
}

func (a app) mustRun(t *testing.T, args ...string) string {
	t.Helper()
	out, err := a.run(t, args...)
	if err != nil {
		t.Fatalf("%v: %q, %v", args, out, err)
	}
	return out
}

func wantOut(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("salida = %q, se esperaba %q", got, want)
	}
}

func wantErr(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, se esperaba %q", err, want)
	}
}

func TestEditEntry(t *testing.T) {
	a := newApp(t)
	a.mustRun(t, "start", "Informe", "-p", "Trabajo")
	wantOut(t, a.mustRun(t, "edit", "1", "-t", "Informe final", "-p", "Otro", "-d", "nota"), "editado #1 Informe final\n")
	var entry map[string]any
	if err := json.Unmarshal([]byte(a.mustRun(t, "edit", "1", "-d", "otra", "--json")), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["title"] != "Informe final" || entry["project"] != "Otro" || entry["description"] != "otra" || entry["uid"] == "" {
		t.Fatalf("edit JSON = %#v", entry)
	}
	_, err := a.run(t, "edit", "1", "-t", "  ")
	wantErr(t, err, "el título no puede estar vacío")
	_, err = a.run(t, "edit", "99", "-t", "x")
	wantErr(t, err, "la tarea no existe")
	_, err = a.run(t, "edit", "1")
	wantErr(t, err, "uso: nexus edit <id> [-t título] [-p proyecto] [-d descripción] [--json]")
}

func TestRemoveRestoreAndTrash(t *testing.T) {
	a := newApp(t)
	a.mustRun(t, "start", "Informe", "-p", "Trabajo")
	wantOut(t, a.mustRun(t, "trash"), "Papelera vacía\n")
	wantOut(t, a.mustRun(t, "rm", "1"), "eliminado #1 (restaurar: nexus restore 1)\n")
	if out := a.mustRun(t, "status"); !strings.HasPrefix(out, "En curso: sin temporizadores") {
		t.Fatalf("el temporizador debía detenerse: %q", out)
	}
	if out := a.mustRun(t, "trash"); !strings.HasPrefix(out, "#1 Informe (Trabajo) · borrada ") {
		t.Fatalf("trash = %q", out)
	}
	var trash []map[string]any
	if err := json.Unmarshal([]byte(a.mustRun(t, "trash", "--json")), &trash); err != nil || len(trash) != 1 || trash[0]["title"] != "Informe" {
		t.Fatalf("trash JSON: %v %v", trash, err)
	}
	wantOut(t, a.mustRun(t, "restore", "1"), "restaurado #1\n")
	wantOut(t, a.mustRun(t, "trash"), "Papelera vacía\n")
	wantOut(t, a.mustRun(t, "rm", "1", "--json"), "{\"ok\":true}\n")
	_, err := a.run(t, "restore", "7")
	wantErr(t, err, "la tarea no existe")
	_, err = a.run(t, "rm", "abc")
	wantErr(t, err, "id inválido «abc»")
}

func TestAdditiveEntryJSONFields(t *testing.T) {
	a := newApp(t)
	a.mustRun(t, "start", "Informe", "-d", "nota")
	var status struct{ Running []map[string]any }
	if err := json.Unmarshal([]byte(a.mustRun(t, "status", "--json")), &status); err != nil {
		t.Fatal(err)
	}
	var list struct{ Running, Recent []map[string]any }
	if err := json.Unmarshal([]byte(a.mustRun(t, "ls", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	for _, e := range []map[string]any{status.Running[0], list.Running[0], list.Recent[0]} {
		if e["uid"] == nil || e["uid"] == "" || e["description"] != "nota" || e["title"] != "Informe" {
			t.Fatalf("faltan campos aditivos: %#v", e)
		}
	}
}

func TestOrganizationAndClientCommands(t *testing.T) {
	a := newApp(t)
	wantOut(t, a.mustRun(t, "org", "add", "Acme"), "organización creada #1 Acme\n")
	wantOut(t, a.mustRun(t, "client", "add", "Globex", "-o", "acme"), "cliente creado #1 Globex\n")
	wantOut(t, a.mustRun(t, "org", "rename", "ACME", "Acme SA"), "organización renombrada #1 Acme SA\n")
	wantOut(t, a.mustRun(t, "client", "rename", "1", "Globex SA"), "cliente renombrado #1 Globex SA\n")
	wantOut(t, a.mustRun(t, "client", "mv", "globex sa", "-"), "cliente movido #1 Globex SA: sin organización\n")
	wantOut(t, a.mustRun(t, "client", "mv", "Globex SA", "1"), "cliente movido #1 Globex SA: organización «Acme SA»\n")
	var client map[string]any
	if err := json.Unmarshal([]byte(a.mustRun(t, "client", "add", "Initech", "--json")), &client); err != nil {
		t.Fatal(err)
	}
	if client["name"] != "Initech" || client["uid"] == "" || client["organization_id"] != nil {
		t.Fatalf("client JSON = %#v", client)
	}
	_, err := a.run(t, "org", "add", " ")
	wantErr(t, err, "el nombre no puede estar vacío")
	_, err = a.run(t, "client", "add", "X", "-o", "nada")
	wantErr(t, err, "la organización no existe")
	_, err = a.run(t, "client", "rename", "99", "Y")
	wantErr(t, err, "el cliente no existe")
	_, err = a.run(t, "org", "add", "acme sa")
	wantErr(t, err, "ya existe una organización con ese nombre")
}

func TestProjectCommands(t *testing.T) {
	a := newApp(t)
	a.mustRun(t, "client", "add", "Globex")
	wantOut(t, a.mustRun(t, "project", "add", "Web", "-c", "globex"), "proyecto creado #1 Web\n")
	wantOut(t, a.mustRun(t, "project", "rename", "web", "Sitio"), "proyecto renombrado #1 Sitio\n")
	wantOut(t, a.mustRun(t, "project", "mv", "Sitio", "-"), "proyecto movido #1 Sitio: sin cliente\n")
	wantOut(t, a.mustRun(t, "project", "mv", "1", "Globex"), "proyecto movido #1 Sitio: cliente «Globex»\n")
	wantOut(t, a.mustRun(t, "project", "archive", "Sitio"), "proyecto archivado #1 Sitio\n")
	wantOut(t, a.mustRun(t, "project", "unarchive", "Sitio"), "proyecto desarchivado #1 Sitio\n")
	var project map[string]any
	if err := json.Unmarshal([]byte(a.mustRun(t, "project", "archive", "Sitio", "--json")), &project); err != nil {
		t.Fatal(err)
	}
	if project["name"] != "Sitio" || project["archived"] != true || project["client_id"] != float64(1) {
		t.Fatalf("project JSON = %#v", project)
	}
	a.mustRun(t, "project", "add", "App", "-c", "Globex")
	_, err := a.run(t, "project", "rename", "App", "sitio")
	wantErr(t, err, "ya existe un proyecto con ese nombre; usa «nexus project merge» para unirlos")
	_, err = a.run(t, "project", "merge", "App", "app", "--yes")
	wantErr(t, err, "no se puede unir un proyecto consigo mismo")
}

func TestReferenceNameWinsOverID(t *testing.T) {
	a := newApp(t)
	a.mustRun(t, "client", "add", "uno")  // id 1
	a.mustRun(t, "client", "add", "2")    // id 2, nombre numérico
	a.mustRun(t, "client", "add", "tres") // id 3
	a.mustRun(t, "client", "rename", "2", "dos")
	// "2" ya no es un nombre: se resuelve por id.
	wantOut(t, a.mustRun(t, "client", "rename", "2", "dos-nuevo"), "cliente renombrado #2 dos-nuevo\n")
	a.mustRun(t, "client", "rename", "3", "1") // el cliente #3 se llama "1"
	// "1" coincide con el nombre del cliente #3, que gana sobre el id 1.
	wantOut(t, a.mustRun(t, "client", "rename", "1", "uno-nuevo"), "cliente renombrado #3 uno-nuevo\n")
}

func TestDestructiveCommandsRequireYes(t *testing.T) {
	a := newApp(t)
	a.mustRun(t, "org", "add", "Acme")
	a.mustRun(t, "client", "add", "Globex", "-o", "Acme")
	a.mustRun(t, "project", "add", "Web", "-c", "Globex")
	a.mustRun(t, "project", "add", "Api")
	a.mustRun(t, "start", "Tarea", "-p", "Web")
	before := a.mustRun(t, "tree", "--json")
	cases := [][]string{
		{"org", "rm", "Acme"},
		{"client", "rm", "Globex"},
		{"project", "rm", "Web"},
		{"project", "merge", "Web", "Api"},
	}
	wants := []string{
		"Esto eliminará la organización «Acme»; sus clientes quedarán sin organización. Repite con --yes para confirmar.",
		"Esto eliminará el cliente «Globex»; sus proyectos quedarán sin cliente. Repite con --yes para confirmar.",
		"Esto eliminará el proyecto «Web»; 1 tarea quedará sin proyecto. Repite con --yes para confirmar.",
		"Esto unirá el proyecto «Web» en «Api»; 1 tarea pasará a «Api» y «Web» dejará de existir. Repite con --yes para confirmar.",
	}
	for i, args := range cases {
		out, err := a.run(t, args...)
		wantErr(t, err, wants[i])
		wantOut(t, out, "")
		if after := a.mustRun(t, "tree", "--json"); after != before {
			t.Fatalf("%v modificó el catálogo sin --yes", args)
		}
	}
	if out := a.mustRun(t, "status"); !strings.Contains(out, "Tarea") {
		t.Fatalf("la tarea debía seguir en curso: %q", out)
	}
	out, err := a.run(t, "project", "rm", "Web", "--json")
	if err == nil || !strings.Contains(out, `"error":"Esto eliminará el proyecto`) {
		t.Fatalf("--json sin --yes = %q, %v", out, err)
	}
}

func TestDestructiveCommandsWithYes(t *testing.T) {
	a := newApp(t)
	a.mustRun(t, "org", "add", "Acme")
	a.mustRun(t, "client", "add", "Globex", "-o", "Acme")
	a.mustRun(t, "project", "add", "Web", "-c", "Globex")
	a.mustRun(t, "project", "add", "Api")
	a.mustRun(t, "start", "Tarea", "-p", "Web")
	wantOut(t, a.mustRun(t, "project", "merge", "Web", "Api", "--yes"), "proyecto «Web» unido en «Api»\n")
	if out := a.mustRun(t, "ls", "--json"); !strings.Contains(out, `"project":"Api"`) {
		t.Fatalf("la tarea debía pasar a Api: %s", out)
	}
	wantOut(t, a.mustRun(t, "project", "rm", "Api", "--yes"), "proyecto «Api» eliminado\n")
	wantOut(t, a.mustRun(t, "client", "rm", "Globex", "--yes"), "cliente «Globex» eliminado\n")
	wantOut(t, a.mustRun(t, "org", "rm", "Acme", "--yes", "--json"), "{\"ok\":true}\n")
	_, err := a.run(t, "org", "rm", "Acme", "--yes")
	wantErr(t, err, "la organización no existe")
}

func TestTreeText(t *testing.T) {
	a := newApp(t)
	a.mustRun(t, "org", "add", "Acme")
	a.mustRun(t, "client", "add", "Globex", "-o", "Acme")
	a.mustRun(t, "client", "add", "Suelto")
	a.mustRun(t, "project", "add", "Web", "-c", "Globex")
	a.mustRun(t, "project", "add", "Vieja", "-c", "Globex")
	a.mustRun(t, "project", "add", "Libre")
	a.mustRun(t, "project", "archive", "Vieja")
	want := "Acme\n  Globex\n    Web\nSin organización\n  Suelto\n  Sin cliente\n    Libre\n"
	wantOut(t, a.mustRun(t, "tree"), want)
	want = "Acme\n  Globex\n    Vieja (archivado)\n    Web\nSin organización\n  Suelto\n  Sin cliente\n    Libre\n"
	wantOut(t, a.mustRun(t, "tree", "--all"), want)
}

func TestTreeJSON(t *testing.T) {
	a := newApp(t)
	a.mustRun(t, "org", "add", "Acme")
	a.mustRun(t, "client", "add", "Globex", "-o", "Acme")
	a.mustRun(t, "project", "add", "Web", "-c", "Globex")
	a.mustRun(t, "project", "add", "Libre")
	var tree []struct {
		Organization *struct {
			ID   int64
			UID  string
			Name string
		}
		Clients []struct {
			Client   *struct{ Name string }
			Projects []struct {
				Name     string
				Archived bool
			}
		}
	}
	if err := json.Unmarshal([]byte(a.mustRun(t, "tree", "--json")), &tree); err != nil {
		t.Fatal(err)
	}
	if len(tree) != 2 || tree[0].Organization == nil || tree[0].Organization.Name != "Acme" || tree[0].Organization.UID == "" ||
		tree[0].Clients[0].Projects[0].Name != "Web" || tree[1].Organization != nil || tree[1].Clients[0].Client != nil || tree[1].Clients[0].Projects[0].Name != "Libre" {
		t.Fatalf("tree JSON = %+v", tree)
	}
	empty := newApp(t)
	wantOut(t, empty.mustRun(t, "tree", "--json"), "[]\n")
	wantOut(t, empty.mustRun(t, "tree"), "Sin elementos en el catálogo\n")
}

func TestHelpAndUnknownCommand(t *testing.T) {
	a := newApp(t)
	out := a.mustRun(t, "help")
	for _, want := range []string{"Temporizadores", "Tareas", "Catálogo", "nexus project merge", "nexus restore", "nexus tree"} {
		if !strings.Contains(out, want) {
			t.Errorf("la ayuda no contiene %q:\n%s", want, out)
		}
	}
	var stdout, stderr bytes.Buffer
	err := RunWithCatalog([]string{"bogus"}, a.tracker, a.catalog, &stdout, &stderr)
	wantErr(t, err, `comando desconocido "bogus"`)
	if !strings.Contains(stderr.String(), "Catálogo") {
		t.Fatalf("el uso debía imprimirse en stderr: %q", stderr.String())
	}
}

func TestCatalogUnavailable(t *testing.T) {
	_, err := invoke(t, testTracker(t), "tree")
	wantErr(t, err, "el catálogo no está disponible")
}

func TestProjectsMergesCatalogAndKeepsJSONContract(t *testing.T) {
	a := newApp(t)
	a.mustRun(t, "org", "add", "Holinsys")
	a.mustRun(t, "client", "add", "Depilab", "-o", "Holinsys")
	a.mustRun(t, "project", "add", "Depiloto", "-c", "Depilab")
	a.mustRun(t, "project", "add", "Lumirecon", "-c", "Depilab")
	a.mustRun(t, "project", "add", "Viejo")
	a.mustRun(t, "project", "archive", "Viejo")
	a.mustRun(t, "start", "Informe", "-p", "Depiloto")
	a.mustRun(t, "start", "Suelta", "-p", "Texto libre")
	wantOut(t, a.mustRun(t, "projects"), "Depilab/Depiloto\nTexto libre\nDepilab/Lumirecon\n")
	wantOut(t, a.mustRun(t, "projects", "-q", "depilab"), "Depilab/Depiloto\nDepilab/Lumirecon\n")
	var projects []map[string]any
	if err := json.Unmarshal([]byte(a.mustRun(t, "projects", "--json")), &projects); err != nil || len(projects) != 3 {
		t.Fatalf("projects JSON: %v %v", projects, err)
	}
	if _, ok := projects[0]["id"]; !ok {
		t.Fatalf("falta id en projects JSON: %#v", projects[0])
	}
	first := projects[0]
	if first["name"] != "Depiloto" || first["client"] != "Depilab" || first["organization"] != "Holinsys" {
		t.Fatalf("campos aditivos = %#v", first)
	}
	if _, ok := first["last_used"]; !ok {
		t.Fatalf("falta last_used: %#v", first)
	}
	if _, ok := first["seconds"]; !ok {
		t.Fatalf("falta seconds: %#v", first)
	}
	for _, p := range projects {
		if p["name"] == "Texto libre" {
			if _, ok := p["client"]; ok {
				t.Fatalf("client debe omitirse cuando está vacío: %#v", p)
			}
			if _, ok := p["organization"]; ok {
				t.Fatalf("organization debe omitirse cuando está vacío: %#v", p)
			}
		}
	}
}

func TestResumeContinuesSameTask(t *testing.T) {
	a := newApp(t)
	a.mustRun(t, "start", "Informe", "-p", "Trabajo", "-d", "nota")
	a.mustRun(t, "stop", "1")
	wantOut(t, a.mustRun(t, "resume", "1"), "reanudado «Informe» (sesión #2, total 0:00:00)\n")
	_, err := a.run(t, "resume", "1")
	wantErr(t, err, "«Informe» ya está en curso")
	out, err := a.run(t, "resume", "2", "--json")
	wantErr(t, err, "«Informe» ya está en curso")
	wantOut(t, out, "{\"error\":\"«Informe» ya está en curso\"}\n")
	a.mustRun(t, "stop", "2")
	var created map[string]any
	if err := json.Unmarshal([]byte(a.mustRun(t, "resume", "1", "--json")), &created); err != nil {
		t.Fatal(err)
	}
	if created["id"] != float64(3) || created["title"] != "Informe" || created["project"] != "Trabajo" || created["description"] != "nota" || created["copy_of"] != float64(1) || created["task_uid"] == "" || created["total_seconds"] != float64(0) {
		t.Fatalf("resume JSON = %#v", created)
	}
}

func TestTotalClockFormat(t *testing.T) {
	for seconds, want := range map[int64]string{0: "0:00:00", 8107: "2:15:07", 59: "0:00:59", 360000: "100:00:00", -5: "0:00:00"} {
		if got := totalClock(seconds); got != want {
			t.Errorf("totalClock(%d) = %q, quería %q", seconds, got, want)
		}
	}
}

func TestListJSONAddsTaskUIDAndTasks(t *testing.T) {
	a := newApp(t)
	a.mustRun(t, "start", "Informe", "-p", "Trabajo")
	a.mustRun(t, "stop", "1")
	a.mustRun(t, "resume", "1")
	var out struct {
		Running []map[string]any `json:"running"`
		Recent  []map[string]any `json:"recent"`
		Tasks   []map[string]any `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(a.mustRun(t, "ls", "--json")), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Recent) != 2 || len(out.Running) != 1 || len(out.Tasks) != 1 {
		t.Fatalf("ls JSON = %+v", out)
	}
	task := out.Tasks[0]
	if out.Recent[0]["task_uid"] == "" || out.Recent[0]["task_uid"] != out.Recent[1]["task_uid"] || out.Running[0]["task_uid"] != task["task_uid"] {
		t.Fatalf("task_uid no coincide: %+v", out)
	}
	if task["title"] != "Informe" || task["project"] != "Trabajo" || task["running"] != true || task["session_count"] != float64(2) || task["running_entry_id"] != float64(2) || task["last_entry_id"] != float64(2) {
		t.Fatalf("tarea = %#v", task)
	}
	for _, key := range []string{"total_seconds", "last_activity", "description"} {
		if _, ok := task[key]; !ok {
			t.Errorf("falta %s en %#v", key, task)
		}
	}
	// Los campos existentes siguen ahí.
	for _, key := range []string{"id", "title", "project", "started_at", "ended_at", "seconds", "uid", "description"} {
		if _, ok := out.Recent[0][key]; !ok {
			t.Errorf("falta el campo existente %s", key)
		}
	}
}

func TestResumeErrorsAreSpanish(t *testing.T) {
	a := newApp(t)
	_, err := a.run(t, "resume", "99")
	wantErr(t, err, "la tarea no existe")
	_, err = a.run(t, "resume")
	wantErr(t, err, "uso: nexus resume <id> [--json]")
	_, err = a.run(t, "resume", "abc")
	wantErr(t, err, "id inválido «abc»")
	out, err := a.run(t, "resume", "99", "--json")
	wantErr(t, err, "la tarea no existe")
	wantOut(t, out, "{\"error\":\"la tarea no existe\"}\n")
}

func TestUsageMentionsResume(t *testing.T) {
	if !strings.Contains(usageText, "nexus resume <id> [--json]") || !strings.Contains(usageText, "misma tarea") {
		t.Fatal("el uso debe documentar que resume continúa la misma tarea")
	}
}

func TestAmbiguousBareNameListsCandidates(t *testing.T) {
	a := newApp(t)
	a.mustRun(t, "client", "add", "sonrisa de colores")
	a.mustRun(t, "client", "add", "inspira salud")
	a.mustRun(t, "project", "add", "diseno de web", "-c", "sonrisa de colores")
	a.mustRun(t, "project", "add", "diseno de web", "-c", "inspira salud")

	wantErrMsg := "«diseno de web» existe en varios clientes: #1 sonrisa de colores/diseno de web, #2 inspira salud/diseno de web. Usa -p \"cliente/proyecto\" o -p #id."

	// start -p
	_, err := a.run(t, "start", "Nueva tarea", "-p", "diseno de web")
	wantErr(t, err, wantErrMsg)

	// Iniciar una tarea para luego editarla
	a.mustRun(t, "start", "Tarea existente", "-p", "sonrisa de colores/diseno de web")

	// edit -p
	_, err = a.run(t, "edit", "1", "-p", "diseno de web")
	wantErr(t, err, wantErrMsg)

	// project archive
	_, err = a.run(t, "project", "archive", "diseno de web")
	wantErr(t, err, wantErrMsg)

	// project unarchive
	_, err = a.run(t, "project", "unarchive", "diseno de web")
	wantErr(t, err, wantErrMsg)

	// project rename
	_, err = a.run(t, "project", "rename", "diseno de web", "nuevo")
	wantErr(t, err, wantErrMsg)

	// project mv
	_, err = a.run(t, "project", "mv", "diseno de web", "-")
	wantErr(t, err, wantErrMsg)

	// project rm
	_, err = a.run(t, "project", "rm", "diseno de web", "--yes")
	wantErr(t, err, wantErrMsg)

	// project merge
	_, err = a.run(t, "project", "merge", "diseno de web", "#1", "--yes")
	wantErr(t, err, wantErrMsg)
}

func TestProjectRefClientProjectAndHashID(t *testing.T) {
	a := newApp(t)
	a.mustRun(t, "client", "add", "sonrisa de colores")
	a.mustRun(t, "client", "add", "inspira salud")
	a.mustRun(t, "project", "add", "diseno de web", "-c", "sonrisa de colores") // #1
	a.mustRun(t, "project", "add", "diseno de web", "-c", "inspira salud")      // #2
	a.mustRun(t, "project", "add", "libre")                                     // #3 sin cliente

	// start -p con "cliente/proyecto"
	wantOut(t, a.mustRun(t, "start", "T1", "-p", "sonrisa de colores/diseno de web"), "iniciado #1 T1\n")
	// start -p con "#id"
	wantOut(t, a.mustRun(t, "start", "T2", "-p", "#2"), "iniciado #2 T2\n")
	// start -p con bare number
	wantOut(t, a.mustRun(t, "start", "T3", "-p", "2"), "iniciado #3 T3\n")
	// start -p con "-/proyecto"
	wantOut(t, a.mustRun(t, "start", "T4", "-p", "-/libre"), "iniciado #4 T4\n")

	// edit -p con "cliente/proyecto"
	wantOut(t, a.mustRun(t, "edit", "1", "-p", "inspira salud/diseno de web"), "editado #1 T1\n")
	// edit -p con "#id"
	wantOut(t, a.mustRun(t, "edit", "1", "-p", "#1"), "editado #1 T1\n")
	// edit -p con bare number
	wantOut(t, a.mustRun(t, "edit", "1", "-p", "2"), "editado #1 T1\n")

	// project rename con "cliente/proyecto" y con "#id"
	wantOut(t, a.mustRun(t, "project", "rename", "sonrisa de colores/diseno de web", "web sonrisa"), "proyecto renombrado #1 web sonrisa\n")
	wantOut(t, a.mustRun(t, "project", "rename", "#2", "web inspira"), "proyecto renombrado #2 web inspira\n")

	// project mv con "#id" y con "cliente/proyecto"
	wantOut(t, a.mustRun(t, "project", "mv", "#1", "-"), "proyecto movido #1 web sonrisa: sin cliente\n")
	wantOut(t, a.mustRun(t, "project", "mv", "-/web sonrisa", "sonrisa de colores"), "proyecto movido #1 web sonrisa: cliente «sonrisa de colores»\n")

	// project merge con "#id"
	wantOut(t, a.mustRun(t, "project", "merge", "#1", "#2", "--yes"), "proyecto «web sonrisa» unido en «web inspira»\n")

	// project rm con "#id"
	wantOut(t, a.mustRun(t, "project", "rm", "#2", "--yes"), "proyecto «web inspira» eliminado\n")
}

func TestProjectsJSONIncludesIDAndTextShowsClientPrefix(t *testing.T) {
	a := newApp(t)
	a.mustRun(t, "client", "add", "Acme")
	a.mustRun(t, "project", "add", "Web", "-c", "Acme")
	a.mustRun(t, "project", "add", "Libre")

	// Texto: muestra "cliente/proyecto" cuando tiene cliente (orden alfabético por nombre)
	wantOut(t, a.mustRun(t, "projects"), "Libre\nAcme/Web\n")

	// JSON: incluye id y mantiene campos existentes
	var projs []map[string]any
	if err := json.Unmarshal([]byte(a.mustRun(t, "projects", "--json")), &projs); err != nil {
		t.Fatal(err)
	}
	if len(projs) != 2 {
		t.Fatalf("se esperaban 2 proyectos, se obtuvo: %#v", projs)
	}
	if projs[0]["id"] != float64(2) || projs[0]["name"] != "Libre" {
		t.Fatalf("proyecto 0 = %#v", projs[0])
	}
	if projs[1]["id"] != float64(1) || projs[1]["name"] != "Web" || projs[1]["client"] != "Acme" {
		t.Fatalf("proyecto 1 = %#v", projs[1])
	}
}

func TestReportAndListShowClientPrefixForDuplicateNames(t *testing.T) {
	a := newApp(t)
	a.mustRun(t, "client", "add", "Cliente A")
	a.mustRun(t, "client", "add", "Cliente B")
	a.mustRun(t, "project", "add", "Comun", "-c", "Cliente A") // #1
	a.mustRun(t, "project", "add", "Comun", "-c", "Cliente B") // #2
	a.mustRun(t, "project", "add", "Unico", "-c", "Cliente A") // #3

	a.mustRun(t, "start", "T1", "-p", "#1")
	*a.now = a.now.Add(10 * time.Minute)
	a.mustRun(t, "stop", "1")

	a.mustRun(t, "start", "T2", "-p", "#2")
	*a.now = a.now.Add(10 * time.Minute)
	a.mustRun(t, "stop", "2")

	a.mustRun(t, "start", "T3", "-p", "#3")
	*a.now = a.now.Add(10 * time.Minute)
	a.mustRun(t, "stop", "3")

	// report muestra cliente/proyecto para nombres duplicados
	reportOut := a.mustRun(t, "report")
	if !strings.Contains(reportOut, "Cliente A/Comun") || !strings.Contains(reportOut, "Cliente B/Comun") {
		t.Fatalf("report debe mostrar cliente/proyecto para nombres duplicados: %s", reportOut)
	}
	if !strings.Contains(reportOut, "Unico") || strings.Contains(reportOut, "Cliente A/Unico") {
		t.Fatalf("report no debe prefijar nombres únicos: %s", reportOut)
	}

	// ls --json muestra cliente/proyecto para nombres duplicados
	var list struct {
		Recent []map[string]any `json:"recent"`
		Tasks  []map[string]any `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(a.mustRun(t, "ls", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	for _, r := range list.Recent {
		if r["title"] == "T1" && r["project"] != "Cliente A/Comun" {
			t.Fatalf("recent T1 project = %v, se esperaba Cliente A/Comun", r["project"])
		}
		if r["title"] == "T2" && r["project"] != "Cliente B/Comun" {
			t.Fatalf("recent T2 project = %v, se esperaba Cliente B/Comun", r["project"])
		}
		if r["title"] == "T3" && r["project"] != "Unico" {
			t.Fatalf("recent T3 project = %v, se esperaba Unico", r["project"])
		}
	}
}
