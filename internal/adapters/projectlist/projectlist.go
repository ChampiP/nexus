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
	ID           int64
	Name         string
	Client       string
	ClientID     int64
	Organization string
	LastUsed     int64
	Seconds      int64
}

// GroupKind identifica una fila de encabezado o una opción de proyecto.
type GroupKind int

const (
	OrganizationGroup GroupKind = iota
	ClientGroup
	ProjectGroup
)

func (k GroupKind) String() string {
	switch k {
	case OrganizationGroup:
		return "organization"
	case ClientGroup:
		return "client"
	default:
		return "project"
	}
}

// Group es una fila de la jerarquía; solo ProjectGroup contiene una opción seleccionable.
type Group struct {
	Kind   GroupKind
	Name   string
	Option Option
}

// Label es el nombre con el cliente cuando lo tiene: «Depiloto · Depilab».
func (o Option) Label() string {
	if o.Client == "" {
		return o.Name
	}
	return o.Name + " · " + o.Client
}

// Build devuelve los proyectos no archivados del catálogo (con o sin tareas) más los proyectos
// que solo existen como texto en las tareas, filtrados por query en la jerarquía.
// Orden: usados recientemente primero; luego los nunca usados por nombre.
func Build(tree []catalog.TreeOrganization, usage []tracking.ProjectUsage, query string) []Option {
	usedByID := make(map[int64]tracking.ProjectUsage)
	usedByName := make(map[string]tracking.ProjectUsage)
	for _, u := range usage {
		if u.ProjectID > 0 {
			usedByID[u.ProjectID] = u
		} else {
			usedByName[strings.ToLower(u.Name)] = u
		}
	}
	seenID := make(map[int64]bool)
	seenName := make(map[string]bool)
	var options []Option
	for _, group := range tree {
		for _, client := range group.Clients {
			for _, project := range client.Projects {
				seenID[project.ID] = true
				seenName[strings.ToLower(project.Name)] = true
				if project.ArchivedAt != nil {
					continue
				}
				option := Option{
					ID:   project.ID,
					Name: project.Name,
				}
				if u, ok := usedByID[project.ID]; ok {
					option.LastUsed = u.LastUsed
					option.Seconds = u.Seconds
				} else if u, ok := usedByName[strings.ToLower(project.Name)]; ok {
					option.LastUsed = u.LastUsed
					option.Seconds = u.Seconds
				}
				if client.Client != nil {
					option.Client = client.Client.Name
					option.ClientID = client.Client.ID
				}
				if group.Organization != nil {
					option.Organization = group.Organization.Name
				}
				options = append(options, option)
			}
		}
	}
	for _, u := range usage {
		if u.ProjectID > 0 {
			if !seenID[u.ProjectID] && strings.TrimSpace(u.Name) != "" {
				options = append(options, Option{ID: u.ProjectID, Name: u.Name, LastUsed: u.LastUsed, Seconds: u.Seconds})
			}
		} else {
			if !seenName[strings.ToLower(u.Name)] && strings.TrimSpace(u.Name) != "" {
				options = append(options, Option{Name: u.Name, LastUsed: u.LastUsed, Seconds: u.Seconds})
			}
		}
	}
	query = strings.ToLower(strings.TrimSpace(query))
	filtered := options[:0]
	for _, option := range options {
		if strings.Contains(strings.ToLower(option.Name), query) || strings.Contains(strings.ToLower(option.Client), query) || strings.Contains(strings.ToLower(option.Organization), query) {
			filtered = append(filtered, option)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		if a.LastUsed != b.LastUsed {
			return a.LastUsed > b.LastUsed
		}
		if !strings.EqualFold(a.Name, b.Name) {
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}
		return a.ID < b.ID
	})
	return filtered
}

// BuildGroups devuelve filas jerárquicas y mantiene visibles los encabezados de resultados filtrados.
// Los grupos sin organización y sin cliente aparecen al final.
func BuildGroups(tree []catalog.TreeOrganization, usage []tracking.ProjectUsage, query string) []Group {
	projects := Build(tree, usage, query)
	byPath := make(map[string][]Option)
	for _, project := range projects {
		key := project.Organization + "\x00" + project.Client
		byPath[key] = append(byPath[key], project)
	}
	var result []Group
	for _, organization := range tree {
		orgName := ""
		if organization.Organization != nil {
			orgName = organization.Organization.Name
		}
		if orgName == "" {
			continue
		}
		orgGroups := make([]Group, 0)
		for _, client := range organization.Clients {
			clientName := ""
			if client.Client != nil {
				clientName = client.Client.Name
			}
			items := byPath[orgName+"\x00"+clientName]
			if len(items) == 0 {
				continue
			}
			if clientName != "" {
				orgGroups = append(orgGroups, Group{Kind: ClientGroup, Name: clientName})
			}
			for _, item := range items {
				orgGroups = append(orgGroups, Group{Kind: ProjectGroup, Name: item.Name, Option: item})
			}
			delete(byPath, orgName+"\x00"+clientName)
		}
		if len(orgGroups) > 0 {
			result = append(result, Group{Kind: OrganizationGroup, Name: orgName})
			result = append(result, orgGroups...)
		}
	}
	var noOrg []Option
	for key, items := range byPath {
		if strings.HasPrefix(key, "\x00") {
			noOrg = append(noOrg, items...)
			delete(byPath, key)
		}
	}
	// Opciones sin organización se agrupan por cliente; proyectos históricos sueltos no tienen cliente.
	clients := map[string][]Option{}
	for _, item := range noOrg {
		clients[item.Client] = append(clients[item.Client], item)
	}
	if len(clients) > 0 {
		result = append(result, Group{Kind: OrganizationGroup, Name: "Sin organización"})
		clientNames := make([]string, 0, len(clients))
		for name := range clients {
			clientNames = append(clientNames, name)
		}
		sort.Strings(clientNames)
		for _, name := range clientNames {
			if name == "" {
				result = append(result, Group{Kind: ClientGroup, Name: "Sin cliente"})
			} else {
				result = append(result, Group{Kind: ClientGroup, Name: name})
			}
			for _, item := range clients[name] {
				result = append(result, Group{Kind: ProjectGroup, Name: item.Name, Option: item})
			}
		}
	}
	return result
}
