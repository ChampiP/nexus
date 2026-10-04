// Package archtest enforces the module boundaries described in docs/ARCHITECTURE.md.
package archtest

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	tracking  = "nexus/internal/tracking"
	catalog   = "nexus/internal/catalog"
	countdown = "nexus/internal/countdown"
	wellbeing = "nexus/internal/wellbeing"
	presence  = "nexus/internal/platform/presence"
	platform  = "nexus/internal/platform"
	adapters  = "nexus/internal/adapters"
	cliPkg    = "nexus/internal/adapters/cli"
	tuiPkg    = "nexus/internal/adapters/tui"
	daemonPkg = "nexus/internal/adapters/daemon"
	notifyPkg = "nexus/internal/platform/notify"
	// projectListPkg es compartido por los adaptadores; solo puede depender de tracking y catalog.
	projectListPkg = "nexus/internal/adapters/projectlist"
)

type listedPackage struct {
	ImportPath string
	Dir        string
	Imports    []string
}

// listPackages returns the module's packages and their direct imports.
func listPackages(t *testing.T) []listedPackage {
	t.Helper()
	out, err := exec.Command("go", "list", "-json", "nexus/...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	var packages []listedPackage
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	for decoder.More() {
		var p listedPackage
		if err := decoder.Decode(&p); err != nil {
			t.Fatal(err)
		}
		packages = append(packages, p)
	}
	return packages
}

func within(path, root string) bool { return path == root || strings.HasPrefix(path, root+"/") }

func TestModuleBoundaries(t *testing.T) {
	for _, p := range listPackages(t) {
		for _, imp := range p.Imports {
			switch {
			case within(p.ImportPath, tracking) && within(imp, adapters):
				t.Errorf("%s must not import adapter %s", p.ImportPath, imp)
			case within(p.ImportPath, tracking) && within(imp, catalog), within(p.ImportPath, catalog) && within(imp, tracking):
				t.Errorf("%s must not import %s", p.ImportPath, imp)
			case within(p.ImportPath, countdown) && (within(imp, tracking) || within(imp, catalog) || within(imp, adapters)):
				t.Errorf("%s must not import %s", p.ImportPath, imp)
			case (within(p.ImportPath, tracking) || within(p.ImportPath, catalog)) && within(imp, countdown):
				t.Errorf("%s must not import %s", p.ImportPath, imp)
			case within(p.ImportPath, catalog) && within(imp, adapters):
				t.Errorf("%s must not import adapter %s", p.ImportPath, imp)
			case within(p.ImportPath, platform) && strings.HasPrefix(imp, "nexus/") && !within(imp, platform):
				t.Errorf("%s (platform) must not import %s", p.ImportPath, imp)
			case p.ImportPath == projectListPkg && strings.HasPrefix(imp, "nexus/") && !within(imp, tracking) && !within(imp, catalog):
				t.Errorf("%s solo puede importar tracking y catalog, no %s", p.ImportPath, imp)
			case within(imp, projectListPkg) && p.ImportPath != cliPkg && p.ImportPath != tuiPkg && p.ImportPath != projectListPkg && !strings.HasPrefix(p.ImportPath, "nexus/cmd/") && !strings.HasSuffix(p.ImportPath, "projectlist.test"):
				t.Errorf("%s no debe importar %s", p.ImportPath, imp)
			case within(p.ImportPath, wellbeing) && strings.HasPrefix(imp, "nexus/") && imp != "nexus/internal/platform/db":
				t.Errorf("%s no debe importar módulos ni adaptadores, no %s", p.ImportPath, imp)
			case p.ImportPath == presence && strings.HasPrefix(imp, "nexus/"):
				t.Errorf("%s no debe importar paquetes internos, no %s", p.ImportPath, imp)
			case p.ImportPath == daemonPkg && strings.HasPrefix(imp, "nexus/") && !within(imp, countdown) && !within(imp, wellbeing) && !within(imp, presence) && imp != notifyPkg:
				t.Errorf("%s no puede importar %s", p.ImportPath, imp)
			case p.ImportPath == notifyPkg && strings.HasPrefix(imp, "nexus/"):
				t.Errorf("%s no debe importar paquetes internos, no %s", p.ImportPath, imp)
			case within(imp, daemonPkg) && !strings.HasPrefix(p.ImportPath, "nexus/cmd/") && p.ImportPath != daemonPkg && !strings.HasSuffix(p.ImportPath, "daemon.test"):
				t.Errorf("%s no debe importar %s", p.ImportPath, imp)
			case p.ImportPath == cliPkg && imp == tuiPkg, p.ImportPath == tuiPkg && imp == cliPkg:
				t.Errorf("%s must not import %s", p.ImportPath, imp)
			}
		}
	}
}

func TestDomainImportsOnlyStandardLibrary(t *testing.T) {
	for _, p := range listPackages(t) {
		if p.ImportPath != tracking && p.ImportPath != wellbeing {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(p.Dir, "domain.go"), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if first, _, _ := strings.Cut(path, "/"); strings.Contains(first, ".") {
				t.Errorf("domain.go imports non-standard package %s", path)
			}
			if strings.HasPrefix(path, "nexus/") {
				t.Errorf("domain.go imports module package %s", path)
			}
		}
		return
	}
	t.Fatal("tracking package not found")
}
