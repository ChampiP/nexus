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
