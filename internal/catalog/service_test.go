package catalog

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// fakeEntries records the calls the Service makes to the tracking side.
type fakeEntries struct {
	calls []string
	err   error
}

func (f *fakeEntries) Relink(fromID, toID int64, toName string) error {
	f.calls = append(f.calls, fmt.Sprintf("relink %d %d %q", fromID, toID, toName))
	return f.err
}

func (f *fakeEntries) RenameProject(id int64, name string) error {
	f.calls = append(f.calls, fmt.Sprintf("rename %d %q", id, name))
	return f.err
}

func newService(t *testing.T) (*Service, *fakeEntries) {
	t.Helper()
	entries := &fakeEntries{}
	return NewService(openCatalog(t), entries, func() time.Time { return time.Unix(500, 0) }), entries
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestOrganizationLifecycle(t *testing.T) {
	s, _ := newService(t)
	org, err := s.CreateOrganization("  Holinsys ")
	if err != nil || org.Name != "Holinsys" || org.ID == 0 || len(org.UID) != 26 {
		t.Fatalf("Create = %+v, %v", org, err)
	}
	if _, err := s.CreateOrganization("   "); !errors.Is(err, ErrEmptyName) {
		t.Fatalf("blank = %v", err)
	}
	if _, err := s.CreateOrganization("holinsys"); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("duplicate = %v", err)
	}
	other := must(s.CreateOrganization("Emana"))
	if err := s.RenameOrganization(other.ID, "HOLINSYS"); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("rename duplicate = %v", err)
	}
	if err := s.RenameOrganization(other.ID, " Emana SA "); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameOrganization(999, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rename missing = %v", err)
	}
	if err := s.DeleteOrganization(999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing = %v", err)
	}
}

func TestDeleteOrganizationKeepsClients(t *testing.T) {
	s, _ := newService(t)
	org := must(s.CreateOrganization("Org"))
	client := must(s.CreateClient("Cli", org.ID))
	if client.OrganizationID == nil || *client.OrganizationID != org.ID {
		t.Fatalf("client = %+v", client)
	}
	if err := s.DeleteOrganization(org.ID); err != nil {
		t.Fatal(err)
	}
	tree := must(s.Tree())
	if len(tree) != 1 || tree[0].Organization != nil || len(tree[0].Clients) != 1 || tree[0].Clients[0].Client.Name != "Cli" {
		t.Fatalf("tree = %+v", tree)
	}
}

func TestClientRulesPerOrganization(t *testing.T) {
	s, _ := newService(t)
	a := must(s.CreateOrganization("A"))
	b := must(s.CreateOrganization("B"))
	ca := must(s.CreateClient("Depilab", a.ID))
	if _, err := s.CreateClient("depilab", a.ID); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("dup in org = %v", err)
	}
	// Same name in another organization, and with no organization, is fine.
	cb := must(s.CreateClient("Depilab", b.ID))
	none := must(s.CreateClient("Depilab", 0))
	if _, err := s.CreateClient("DEPILAB", 0); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("dup without org = %v", err)
	}
	if _, err := s.CreateClient("x", 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing org = %v", err)
	}
	if _, err := s.CreateClient(" ", 0); !errors.Is(err, ErrEmptyName) {
		t.Fatalf("blank = %v", err)
	}
	if err := s.MoveClient(cb.ID, a.ID); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("move into dup = %v", err)
	}
	if err := s.MoveClient(none.ID, a.ID); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("move none into dup = %v", err)
	}
	if err := s.MoveClient(ca.ID, 0); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("move to none dup = %v", err)
	}
	if err := s.RenameClient(ca.ID, "Otro"); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameClient(cb.ID, "otro"); err != nil {
		t.Fatalf("same name in other org should work: %v", err)
	}
	if err := s.RenameClient(none.ID, "OTRO"); err != nil {
		t.Fatalf("same name with no org should work: %v", err)
	}
	if err := s.MoveClient(ca.ID, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("move to missing org = %v", err)
	}
	if err := s.MoveClient(999, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("move missing = %v", err)
	}
	if err := s.RenameClient(999, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rename missing = %v", err)
	}
}

func TestDeleteClientKeepsProjects(t *testing.T) {
	s, _ := newService(t)
	c := must(s.CreateClient("C", 0))
	p := must(s.CreateProject("P", c.ID))
	if err := s.DeleteClient(c.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteClient(c.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v", err)
	}
	tree := must(s.Tree())
	if len(tree) != 1 || len(tree[0].Clients) != 1 || tree[0].Clients[0].Client != nil || tree[0].Clients[0].Projects[0].ID != p.ID {
		t.Fatalf("tree = %+v", tree)
	}
}

func TestProjectCreateRenameMoveArchive(t *testing.T) {
	s, entries := newService(t)
	c := must(s.CreateClient("C", 0))
	p := must(s.CreateProject(" Nexus ", c.ID))
	if p.Name != "Nexus" || p.ClientID == nil || *p.ClientID != c.ID {
		t.Fatalf("project = %+v", p)
	}
	if _, err := s.CreateProject("nexus", c.ID); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("dup = %v", err)
	}
	otherClientless := must(s.CreateProject("nexus", 0))
	if otherClientless.Name != "nexus" {
		t.Fatalf("proyecto sin cliente = %+v", otherClientless)
	}
	if _, err := s.CreateProject("", 0); !errors.Is(err, ErrEmptyName) {
		t.Fatalf("blank = %v", err)
	}
	if _, err := s.CreateProject("x", 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing client = %v", err)
	}
	other := must(s.CreateProject("Otro", c.ID))
	if err := s.RenameProject(other.ID, "NEXUS"); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("rename dup = %v", err)
	}
	if len(entries.calls) != 0 {
		t.Fatalf("failed rename must not touch entries: %v", entries.calls)
	}
	if err := s.RenameProject(p.ID, "nexus"); err != nil {
		t.Fatalf("case-only rename: %v", err)
	}
	if err := s.RenameProject(p.ID, "Nexus 2"); err != nil {
		t.Fatal(err)
	}
	if got := entries.calls[len(entries.calls)-1]; got != fmt.Sprintf("rename %d %q", p.ID, "Nexus 2") {
		t.Fatalf("calls = %v", entries.calls)
	}
	if err := s.RenameProject(999, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rename missing = %v", err)
	}
	if err := s.MoveProject(p.ID, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.MoveProject(p.ID, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("move missing client = %v", err)
	}
	if err := s.ArchiveProject(p.ID); err != nil {
		t.Fatal(err)
	}
	tree := must(s.Tree())
	archived := 0
	for _, o := range tree {
		for _, c := range o.Clients {
			for _, pr := range c.Projects {
				if pr.ID == p.ID && pr.ArchivedAt != nil && *pr.ArchivedAt == 500 {
					archived++
				}
			}
		}
	}
	if archived != 1 {
		t.Fatalf("archived flag not in tree: %+v", tree)
	}
	if err := s.UnarchiveProject(p.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ArchiveProject(999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archive missing = %v", err)
	}
}

func TestMergeProjects(t *testing.T) {
	s, entries := newService(t)
	from := must(s.CreateProject("From", 0))
	into := must(s.CreateProject("Into", 0))
	if err := s.MergeProjects(from.ID, from.ID); err == nil {
		t.Fatal("merge into itself must fail")
	}
	if err := s.MergeProjects(from.ID, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("merge into missing = %v", err)
	}
	if len(entries.calls) != 0 {
		t.Fatalf("failed merges touched entries: %v", entries.calls)
	}
	if err := s.MergeProjects(from.ID, into.ID); err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("relink %d %d %q", from.ID, into.ID, "Into"); len(entries.calls) != 1 || entries.calls[0] != want {
		t.Fatalf("calls = %v, want %s", entries.calls, want)
	}
	if err := s.MergeProjects(from.ID, into.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("merge twice = %v", err)
	}
}

func TestDeleteProjectUnlinksEntriesFirst(t *testing.T) {
	s, entries := newService(t)
	p := must(s.CreateProject("P", 0))
	entries.err = errors.New("boom")
	if err := s.DeleteProject(p.ID); err == nil {
		t.Fatal("expected entries failure")
	}
	if len(must(s.Tree())) != 1 {
		t.Fatal("project row must survive a failed entries step")
	}
	entries.err = nil
	entries.calls = nil
	if err := s.DeleteProject(p.ID); err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("relink %d 0 \"\"", p.ID); len(entries.calls) != 1 || entries.calls[0] != want {
		t.Fatalf("calls = %v", entries.calls)
	}
	if got := must(s.Tree()); len(got) != 0 {
		t.Fatalf("tree = %+v", got)
	}
	if err := s.DeleteProject(p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete twice = %v", err)
	}
}

func TestTreeGroupingAndOrder(t *testing.T) {
	s, _ := newService(t)
	zeta := must(s.CreateOrganization("zeta"))
	alpha := must(s.CreateOrganization("Alpha"))
	must(s.CreateOrganization("Empty"))
	cb := must(s.CreateClient("beta", alpha.ID))
	ca := must(s.CreateClient("Apple", alpha.ID))
	loose := must(s.CreateClient("Loose", 0))
	_ = zeta
	must(s.CreateProject("b-proj", ca.ID))
	must(s.CreateProject("A-proj", ca.ID))
	must(s.CreateProject("in-loose", loose.ID))
	must(s.CreateProject("personal", 0))
	_ = cb

	tree := must(s.Tree())
	names := func() string {
		out := ""
		for _, o := range tree {
			if o.Organization == nil {
				out += "[none]"
			} else {
				out += "[" + o.Organization.Name + "]"
			}
			for _, c := range o.Clients {
				if c.Client == nil {
					out += " (no client)"
				} else {
					out += " (" + c.Client.Name + ")"
				}
				for _, p := range c.Projects {
					out += " " + p.Name
				}
			}
		}
		return out
	}()
	want := "[Alpha] (Apple) A-proj b-proj (beta)[Empty][zeta][none] (Loose) in-loose (no client) personal"
	if names != want {
		t.Fatalf("tree = %q\nwant   %q", names, want)
	}
}

func TestProjectsUniquePerClient(t *testing.T) {
	// (1) Mismo nombre bajo distintos clientes permitido.
	s, _ := newService(t)
	c1 := must(s.CreateClient("Cliente 1", 0))
	c2 := must(s.CreateClient("Cliente 2", 0))

	p1 := must(s.CreateProject("Diseno", c1.ID))
	p2, err := s.CreateProject("diseno", c2.ID)
	if err != nil {
		t.Fatalf("mismo nombre bajo dos clientes debe estar permitido: %v", err)
	}
	if p2.ID == p1.ID {
		t.Fatal("los proyectos deben ser distintos")
	}

	// (2) Mismo nombre en el mismo cliente -> ErrDuplicateName (create, rename, move).
	if _, err := s.CreateProject("DISENO", c1.ID); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("create duplicado en el mismo cliente debe fallar con ErrDuplicateName: %v", err)
	}
	other1 := must(s.CreateProject("Otro", c1.ID))
	if err := s.RenameProject(other1.ID, "diseno"); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("rename duplicado en el mismo cliente debe fallar con ErrDuplicateName: %v", err)
	}
	if err := s.MoveProject(p2.ID, c1.ID); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("move hacia cliente con nombre colisionante debe fallar con ErrDuplicateName: %v", err)
	}

	// (3) Duplicados sin cliente -> ErrDuplicateName (create, rename, move).
	pNone := must(s.CreateProject("Sin Cliente", 0))
	if _, err := s.CreateProject("sin cliente", 0); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("create duplicado sin cliente debe fallar con ErrDuplicateName: %v", err)
	}
	otherNone := must(s.CreateProject("Otro Libre", 0))
	if err := s.RenameProject(otherNone.ID, "SIN CLIENTE"); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("rename duplicado sin cliente debe fallar con ErrDuplicateName: %v", err)
	}
	pUnderClient := must(s.CreateProject("Sin Cliente", c1.ID))
	if err := s.MoveProject(pUnderClient.ID, 0); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("move hacia sin cliente con nombre colisionante debe fallar con ErrDuplicateName: %v", err)
	}
	_ = pNone

	// (4) MergeProjects entre clientes distintos funciona correctamente.
	if err := s.MergeProjects(p2.ID, p1.ID); err != nil {
		t.Fatalf("MergeProjects entre proyectos de distintos clientes falló: %v", err)
	}
}

func TestEnsureProjectAndProjectsNamed(t *testing.T) {
	s, _ := newService(t)
	c1 := must(s.CreateClient("Cliente 1", 0))
	c2 := must(s.CreateClient("Cliente 2", 0))

	// EnsureProject(name, client) idempotente y con ámbito por cliente.
	ep1, err := s.repo.EnsureProject("Web", c1.ID)
	if err != nil {
		t.Fatalf("EnsureProject primer cliente: %v", err)
	}
	ep1Again, err := s.repo.EnsureProject("web", c1.ID)
	if err != nil || ep1Again.ID != ep1.ID {
		t.Fatalf("EnsureProject idempotente falló: %v, %v", ep1Again, err)
	}

	ep2, err := s.repo.EnsureProject("Web", c2.ID)
	if err != nil {
		t.Fatalf("EnsureProject segundo cliente: %v", err)
	}
	if ep2.ID == ep1.ID {
		t.Fatal("EnsureProject con diferente cliente debe crear o retornar proyecto independiente")
	}

	// ProjectsNamed lista ambos.
	named, err := s.repo.ProjectsNamed("web")
	if err != nil {
		t.Fatalf("ProjectsNamed: %v", err)
	}
	if len(named) != 2 {
		t.Fatalf("ProjectsNamed esperaba 2 proyectos, obtuvo %d", len(named))
	}

	// Adaptador ambiguo retorna ErrAmbiguousProject.
	_, err = s.repo.EnsureProject("Web")
	if !errors.Is(err, ErrAmbiguousProject) {
		t.Fatalf("wrapper ambiguo con múltiples proyectos debe retornar ErrAmbiguousProject, obtuvo %v", err)
	}

	// EnsureProject sin cliente cuando no existe crea sin cliente.
	epNone, err := s.repo.EnsureProject("UnicoLibre")
	if err != nil {
		t.Fatalf("EnsureProject wrapper único: %v", err)
	}
	if epNone.ClientID != nil {
		t.Fatalf("EnsureProject wrapper nuevo debe ser sin cliente, tiene %v", *epNone.ClientID)
	}
	// Si solo hay uno, el wrapper lo resuelve sin error.
	epNoneAgain, err := s.repo.EnsureProject("unicolibre")
	if err != nil || epNoneAgain.ID != epNone.ID {
		t.Fatalf("EnsureProject wrapper resolución única: %v, %v", epNoneAgain, err)
	}
}
