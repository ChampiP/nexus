package tui

import (
	"reflect"
	"testing"

	"nexus/internal/catalog"
)

func i64(v int64) *int64 { return &v }

func treeFixture() []catalog.TreeOrganization {
	org := catalog.Organization{ID: 1, Name: "Holinsys"}
	depilab := catalog.Client{ID: 1, Name: "Depilab", OrganizationID: i64(1)}
	suelto := catalog.Client{ID: 2, Name: "Suelto"}
	return []catalog.TreeOrganization{
		{Organization: &org, Clients: []catalog.TreeClient{{Client: &depilab, Projects: []catalog.Project{
			{ID: 1, Name: "Depiloto", ClientID: i64(1)},
			{ID: 2, Name: "Viejo", ClientID: i64(1), ArchivedAt: i64(7)},
		}}}},
		{Clients: []catalog.TreeClient{
			{Client: &suelto},
			{Projects: []catalog.Project{{ID: 3, Name: "web suelto"}}},
		}},
	}
}

func rowSummary(rows []catRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = string(rune('0'+r.depth)) + ":" + r.name
	}
	return out
}

func TestBuildCatalogRowsOrderAndGroups(t *testing.T) {
	stats := func(p catalog.Project) (int64, int) { return int64(p.ID) * 60, int(p.ID) }
	rows := buildCatalogRows(treeFixture(), false, stats)
	want := []string{"0:Holinsys", "1:Depilab", "2:Depiloto", "0:Sin organización", "1:Suelto", "1:Sin cliente", "2:web suelto"}
	if got := rowSummary(rows); !reflect.DeepEqual(got, want) {
		t.Fatalf("filas = %v; quiero %v", got, want)
	}
	if rows[3].kind != catHeader || rows[5].kind != catHeader || rows[0].kind != catOrg || rows[1].kind != catClient || rows[2].kind != catProject {
		t.Fatalf("tipos = %+v", rows)
	}
	if rows[2].seconds != 60 || rows[2].tasks != 1 || rows[0].children != 1 || rows[1].children != 2 {
		t.Fatalf("estadísticas = %+v %+v %+v", rows[0], rows[1], rows[2])
	}
	if rows[2].parentID != 1 || rows[1].parentID != 1 {
		t.Fatalf("padres = %+v", rows)
	}
}

func TestBuildCatalogRowsArchivedToggle(t *testing.T) {
	stats := func(catalog.Project) (int64, int) { return 0, 0 }
	rows := buildCatalogRows(treeFixture(), true, stats)
	if len(rows) != 8 || rows[3].name != "Viejo" || !rows[3].archived {
		t.Fatalf("con archivados = %v", rowSummary(rows))
	}
}

func TestBuildCatalogRowsOmitsEmptyUnassignedGroups(t *testing.T) {
	org := catalog.Organization{ID: 1, Name: "Solo"}
	rows := buildCatalogRows([]catalog.TreeOrganization{{Organization: &org}}, false, func(catalog.Project) (int64, int) { return 0, 0 })
	if got := rowSummary(rows); !reflect.DeepEqual(got, []string{"0:Solo"}) {
		t.Fatalf("filas = %v", got)
	}
}

func TestRowActionsPerKind(t *testing.T) {
	cases := []struct {
		row  catRow
		want []string
	}{
		{catRow{kind: catOrg}, []string{"✎ Renombrar", "✕ Eliminar"}},
		{catRow{kind: catClient}, []string{"✎ Renombrar", "⇄ Mover", "✕ Eliminar"}},
		{catRow{kind: catProject}, []string{"✎ Renombrar", "⇄ Mover", "⊕ Unir", "▣ Archivar", "✕ Eliminar"}},
		{catRow{kind: catProject, archived: true}, []string{"✎ Renombrar", "⇄ Mover", "⊕ Unir", "▣ Desarchivar", "✕ Eliminar"}},
		{catRow{kind: catHeader}, []string{}},
	}
	for _, c := range cases {
		if got := rowLabels(c.row); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%+v: %v; quiero %v", c.row, got, c.want)
		}
	}
}

func TestConfirmMessagesShowConsequences(t *testing.T) {
	cases := map[string]string{
		deleteMessage(catRow{kind: catProject, name: "Web", tasks: 12}):   "¿Eliminar el proyecto «Web»? 12 tareas quedarán sin proyecto.",
		deleteMessage(catRow{kind: catProject, name: "Web", tasks: 1}):    "¿Eliminar el proyecto «Web»? 1 tarea quedará sin proyecto.",
		deleteMessage(catRow{kind: catProject, name: "Web"}):              "¿Eliminar el proyecto «Web»? No tiene tareas.",
		deleteMessage(catRow{kind: catClient, name: "Acme", children: 3}): "¿Eliminar el cliente «Acme»? Sus 3 proyectos quedarán sin cliente.",
		deleteMessage(catRow{kind: catOrg, name: "Holi", children: 2}):    "¿Eliminar la organización «Holi»? Sus 2 clientes quedarán sin organización.",
		deleteMessage(catRow{kind: catOrg, name: "Holi", children: 1}):    "¿Eliminar la organización «Holi»? Su cliente quedará sin organización.",
		mergeMessage(catRow{name: "Web", tasks: 12}, "Api"):               "¿Unir «Web» en «Api»? 12 tareas pasarán a «Api».",
		mergeMessage(catRow{name: "Web", tasks: 1}, "Api"):                "¿Unir «Web» en «Api»? 1 tarea pasará a «Api».",
		mergeMessage(catRow{name: "Web"}, "Api"):                          "¿Unir «Web» en «Api»? No tiene tareas que mover.",
		catalogErrorText(catalog.ErrDuplicateName):                        "Ya existe ese nombre; usa Unir para combinar proyectos",
		catalogErrorText(catalog.ErrEmptyName):                            "El nombre no puede estar vacío",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("%q; quiero %q", got, want)
		}
	}
}

func TestFilterPickOptionsMatchesLabelIgnoringCase(t *testing.T) {
	all := []pickOption{{label: "Ninguno"}, {id: 1, label: "Depilab · Holinsys"}, {id: 2, label: "Lumi"}}
	if got := filterPickOptions(all, "  HOLI "); len(got) != 1 || got[0].id != 1 {
		t.Fatalf("filtro = %+v", got)
	}
	if got := filterPickOptions(all, ""); len(got) != 3 {
		t.Fatalf("sin consulta = %+v", got)
	}
}
