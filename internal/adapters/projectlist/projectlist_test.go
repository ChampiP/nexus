package projectlist

import (
	"reflect"
	"testing"

	"nexus/internal/catalog"
	"nexus/internal/tracking"
)

func ptr(v int64) *int64 { return &v }

func sampleTree() []catalog.TreeOrganization {
	org := catalog.Organization{ID: 1, Name: "Holinsys"}
	client := catalog.Client{ID: 1, Name: "Depilab", OrganizationID: ptr(1)}
	return []catalog.TreeOrganization{
		{Organization: &org, Clients: []catalog.TreeClient{{Client: &client, Projects: []catalog.Project{
			{ID: 1, Name: "Depiloto", ClientID: ptr(1)},
			{ID: 2, Name: "Lumirecon", ClientID: ptr(1)},
			{ID: 3, Name: "Viejo", ClientID: ptr(1), ArchivedAt: ptr(5)},
		}}}},
		{Clients: []catalog.TreeClient{{Projects: []catalog.Project{{ID: 4, Name: "web suelto"}}}}},
	}
}

func names(options []Option) []string {
	out := make([]string, len(options))
	for i, o := range options {
		out[i] = o.Name
	}
	return out
}

func TestBuildMergesCatalogWithUsageAndOrders(t *testing.T) {
	usage := []tracking.ProjectUsage{
		{Name: "depiloto", LastUsed: 100, Seconds: 4800},
		{Name: "web suelto", LastUsed: 200, Seconds: 60},
	}
	got := Build(sampleTree(), usage, "")
	if want := []string{"web suelto", "Depiloto", "Lumirecon"}; !reflect.DeepEqual(names(got), want) {
		t.Fatalf("orden = %v; quiero %v", names(got), want)
	}
	if got[1].Client != "Depilab" || got[1].Organization != "Holinsys" || got[1].Seconds != 4800 || got[1].LastUsed != 100 {
		t.Fatalf("Depiloto = %+v", got[1])
	}
	if got[2].LastUsed != 0 || got[2].Seconds != 0 {
		t.Fatalf("Lumirecon sin uso = %+v", got[2])
	}
}

func TestBuildExcludesArchivedEvenWithUsage(t *testing.T) {
	usage := []tracking.ProjectUsage{{Name: "Viejo", LastUsed: 9, Seconds: 9}}
	for _, o := range Build(sampleTree(), usage, "") {
		if o.Name == "Viejo" {
			t.Fatal("un proyecto archivado no debe aparecer")
		}
	}
}

func TestBuildKeepsUsageOnlyProjectsAndNeverUsedByName(t *testing.T) {
	tree := []catalog.TreeOrganization{{Clients: []catalog.TreeClient{{Projects: []catalog.Project{{ID: 1, Name: "zeta"}, {ID: 2, Name: "Alfa"}}}}}}
	got := Build(tree, []tracking.ProjectUsage{{Name: "Suelto", LastUsed: 1}}, "")
	if want := []string{"Suelto", "Alfa", "zeta"}; !reflect.DeepEqual(names(got), want) {
		t.Fatalf("orden = %v; quiero %v", names(got), want)
	}
}

func TestBuildFiltersByProjectOrClientName(t *testing.T) {
	if got := names(Build(sampleTree(), nil, "lumi")); !reflect.DeepEqual(got, []string{"Lumirecon"}) {
		t.Fatalf("por proyecto = %v", got)
	}
	if got := names(Build(sampleTree(), nil, "DEPILAB")); !reflect.DeepEqual(got, []string{"Depiloto", "Lumirecon"}) {
		t.Fatalf("por cliente = %v", got)
	}
}

func TestLabel(t *testing.T) {
	if got := (Option{Name: "Depiloto", Client: "Depilab"}).Label(); got != "Depiloto · Depilab" {
		t.Fatalf("Label = %q", got)
	}
	if got := (Option{Name: "Solo"}).Label(); got != "Solo" {
		t.Fatalf("Label = %q", got)
	}
}

func TestBuildGroupsOrdersAndRetainsHeadersForHierarchySearch(t *testing.T) {
	usage := []tracking.ProjectUsage{{Name: "Depiloto", LastUsed: 10, Seconds: 90}, {Name: "Lumirecon", LastUsed: 20, Seconds: 120}}
	for _, query := range []string{"holinsys", "depilab", "lumirecon"} {
		t.Run(query, func(t *testing.T) {
			groups := BuildGroups(sampleTree(), usage, query)
			var got []string
			for _, group := range groups {
				got = append(got, group.Kind.String()+":"+group.Name)
			}
			want := []string{"organization:Holinsys", "client:Depilab"}
			if query == "holinsys" || query == "depilab" {
				want = append(want, "project:Lumirecon", "project:Depiloto")
			} else {
				want = append(want, "project:Lumirecon")
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("grupos = %v; quiero %v", got, want)
			}
		})
	}
	groups := BuildGroups(sampleTree(), usage, "suelto")
	if len(groups) != 3 || groups[0].Name != "Sin organización" || groups[1].Name != "Sin cliente" || groups[2].Option.Name != "web suelto" {
		t.Fatalf("grupos sin asignar = %+v", groups)
	}
}

func TestBuildAndBuildGroupsSameNamedProjectsDifferentClients(t *testing.T) {
	// Comprueba que dos proyectos con el mismo nombre bajo distintos clientes se reconocen
	// como opciones independientes con sus respectivos identificadores, clientes y totales de segundos.
	c1 := catalog.Client{ID: 10, Name: "Cliente 1"}
	c2 := catalog.Client{ID: 20, Name: "Cliente 2"}
	tree := []catalog.TreeOrganization{
		{
			Clients: []catalog.TreeClient{
				{
					Client: &c1,
					Projects: []catalog.Project{
						{ID: 100, Name: "Diseno de web", ClientID: ptr(10)},
					},
				},
				{
					Client: &c2,
					Projects: []catalog.Project{
						{ID: 200, Name: "Diseno de web", ClientID: ptr(20)},
					},
				},
			},
		},
	}

	usage := []tracking.ProjectUsage{
		{ProjectID: 100, Name: "Diseno de web", Seconds: 1500, LastUsed: 200},
		{ProjectID: 200, Name: "Diseno de web", Seconds: 3500, LastUsed: 300},
	}

	options := Build(tree, usage, "")
	if len(options) != 2 {
		t.Fatalf("Build esperaba 2 opciones, obtuvo %d: %#v", len(options), options)
	}

	byID := map[int64]Option{}
	for _, opt := range options {
		byID[opt.ID] = opt
	}

	opt1, ok1 := byID[100]
	if !ok1 || opt1.Client != "Cliente 1" || opt1.Seconds != 1500 || opt1.LastUsed != 200 {
		t.Fatalf("opción 100 incorrecta: %+v", opt1)
	}

	opt2, ok2 := byID[200]
	if !ok2 || opt2.Client != "Cliente 2" || opt2.Seconds != 3500 || opt2.LastUsed != 300 {
		t.Fatalf("opción 200 incorrecta: %+v", opt2)
	}

	if options[0].ID != 200 || options[1].ID != 100 {
		t.Fatalf("Build orden esperado por LastUsed (200 primero, luego 100), obtuvo: %+v, %+v", options[0], options[1])
	}

	groups := BuildGroups(tree, usage, "")
	var projectGroups []Group
	for _, g := range groups {
		if g.Kind == ProjectGroup {
			projectGroups = append(projectGroups, g)
		}
	}
	if len(projectGroups) != 2 {
		t.Fatalf("BuildGroups esperaba 2 opciones de proyecto, obtuvo %d", len(projectGroups))
	}
	groupsByID := map[int64]Option{}
	for _, pg := range projectGroups {
		groupsByID[pg.Option.ID] = pg.Option
	}
	if g100 := groupsByID[100]; g100.Client != "Cliente 1" || g100.Seconds != 1500 {
		t.Fatalf("grupo id 100 incorrecto: %+v", g100)
	}
	if g200 := groupsByID[200]; g200.Client != "Cliente 2" || g200.Seconds != 3500 {
		t.Fatalf("grupo id 200 incorrecto: %+v", g200)
	}
}
