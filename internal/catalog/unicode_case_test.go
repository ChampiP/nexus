package catalog

import (
	"errors"
	"testing"
)

func TestUnicodeCaseInsensitiveCatalogNames(t *testing.T) {
	t.Run("project uniqueness scope and accents", func(t *testing.T) {
		s, _ := newService(t)
		client := must(s.CreateClient("Cliente", 0))
		otherClient := must(s.CreateClient("Otro cliente", 0))
		must(s.CreateProject("Diseño", client.ID))
		if _, err := s.CreateProject("DISEÑO", client.ID); !errors.Is(err, ErrDuplicateName) {
			t.Fatalf("duplicate Unicode case variant = %v, want ErrDuplicateName", err)
		}
		if _, err := s.CreateProject("DISEÑO", otherClient.ID); err != nil {
			t.Fatalf("same name under another client: %v", err)
		}
		if _, err := s.CreateProject("Diseno", client.ID); err != nil {
			t.Fatalf("accent-insensitive duplicate should be allowed: %v", err)
		}
	})

	t.Run("Unicode lookups", func(t *testing.T) {
		s, _ := newService(t)
		first := must(s.CreateProject("AÑO", 0))
		ensured, err := s.repo.EnsureProject("año", 0)
		if err != nil || ensured.ID != first.ID {
			t.Fatalf("EnsureProject = %+v, %v; want id %d", ensured, err, first.ID)
		}
		must(s.CreateProject("GESTIÓN", 0))
		matches, err := s.ProjectsNamed("gestión")
		if err != nil || len(matches) != 1 || matches[0].Name != "GESTIÓN" {
			t.Fatalf("ProjectsNamed = %+v, %v; want GESTIÓN", matches, err)
		}
	})

	t.Run("rename sibling and own case variant", func(t *testing.T) {
		s, _ := newService(t)
		client := must(s.CreateClient("Cliente", 0))
		sibling := must(s.CreateProject("Gestión", client.ID))
		other := must(s.CreateProject("Otro", client.ID))
		if err := s.RenameProject(other.ID, "GESTIÓN"); !errors.Is(err, ErrDuplicateName) {
			t.Fatalf("rename collision = %v, want ErrDuplicateName", err)
		}
		if err := s.RenameProject(sibling.ID, "GESTIÓN"); err != nil {
			t.Fatalf("rename to own case variant: %v", err)
		}
	})

	t.Run("organization and client uniqueness", func(t *testing.T) {
		s, _ := newService(t)
		org := must(s.CreateOrganization("Diseño"))
		if _, err := s.CreateOrganization("DISEÑO"); !errors.Is(err, ErrDuplicateName) {
			t.Fatalf("organization duplicate = %v, want ErrDuplicateName", err)
		}
		otherOrg := must(s.CreateOrganization("Otro"))
		if err := s.RenameOrganization(otherOrg.ID, "DISEÑO"); !errors.Is(err, ErrDuplicateName) {
			t.Fatalf("organization rename duplicate = %v, want ErrDuplicateName", err)
		}
		client := must(s.CreateClient("Gestión", org.ID))
		if _, err := s.CreateClient("GESTIÓN", org.ID); !errors.Is(err, ErrDuplicateName) {
			t.Fatalf("client duplicate = %v, want ErrDuplicateName", err)
		}
		otherClient := must(s.CreateClient("Otro", org.ID))
		if err := s.RenameClient(otherClient.ID, "GESTIÓN"); !errors.Is(err, ErrDuplicateName) {
			t.Fatalf("client rename duplicate = %v, want ErrDuplicateName", err)
		}
		moveClient := must(s.CreateClient("Gestión", otherOrg.ID))
		if err := s.MoveClient(moveClient.ID, org.ID); !errors.Is(err, ErrDuplicateName) {
			t.Fatalf("client move duplicate = %v, want ErrDuplicateName", err)
		}
		project := must(s.CreateProject("AÑO", moveClient.ID))
		must(s.CreateProject("Año", client.ID))
		if err := s.MoveProject(project.ID, client.ID); !errors.Is(err, ErrDuplicateName) {
			t.Fatalf("project move duplicate = %v, want ErrDuplicateName", err)
		}
	})
}
