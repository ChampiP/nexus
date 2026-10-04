// Package projectlist combina los proyectos del catálogo con el uso registrado para ofrecerlos
// en los selectores (TUI) y en `nexus projects` (CLI), con un único criterio de orden y filtro.
package projectlist

import (
	"sort"
	"strings"

	"nexus/internal/catalog"
	"nexus/internal/tracking"
)

// Option es un proyecto elegible junto con su cliente y su uso.
type Option struct {
	Name         string
	Client       string
	Organization string
	LastUsed     int64
	Seconds      int64
}

// Label es el nombre con el cliente cuando lo tiene: «Depiloto · Depilab».
func (o Option) Label() string {
	if o.Client == "" {
		return o.Name
	}
	return o.Name + " · " + o.Client
}

// Build devuelve los proyectos no archivados del catálogo (con o sin tareas) más los proyectos
// que solo existen como texto en las tareas, filtrados por query contra proyecto o cliente.
// Orden: usados recientemente primero; luego los nunca usados por nombre.
func Build(tree []catalog.TreeOrganization, usage []tracking.ProjectUsage, query string) []Option {
	used := make(map[string]tracking.ProjectUsage, len(usage))
	for _, u := range usage {
		used[strings.ToLower(u.Name)] = u
	}
	seen := map[string]bool{}
	var options []Option
	for _, group := range tree {
		for _, client := range group.Clients {
			for _, project := range client.Projects {
				key := strings.ToLower(project.Name)
				seen[key] = true
				if project.ArchivedAt != nil {
					continue
				}
				option := Option{Name: project.Name, LastUsed: used[key].LastUsed, Seconds: used[key].Seconds}
				if client.Client != nil {
					option.Client = client.Client.Name
				}
				if group.Organization != nil {
					option.Organization = group.Organization.Name
				}
				options = append(options, option)
			}
		}
	}
	for _, u := range usage {
		if !seen[strings.ToLower(u.Name)] && strings.TrimSpace(u.Name) != "" {
			options = append(options, Option{Name: u.Name, LastUsed: u.LastUsed, Seconds: u.Seconds})
		}
	}
	query = strings.ToLower(strings.TrimSpace(query))
	filtered := options[:0]
	for _, option := range options {
		if strings.Contains(strings.ToLower(option.Name), query) || strings.Contains(strings.ToLower(option.Client), query) {
			filtered = append(filtered, option)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		if a.LastUsed != b.LastUsed {
			return a.LastUsed > b.LastUsed
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return filtered
}
