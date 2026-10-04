package cli

import (
	"time"

	"nexus/internal/catalog"
	"nexus/internal/tracking"
)

type statusEntry struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Project     string `json:"project"`
	StartedAt   int64  `json:"started_at"`
	Elapsed     string `json:"elapsed"`
	UID         string `json:"uid"`
	Description string `json:"description"`
	TaskUID     string `json:"task_uid"`
}
type statusOutput struct {
	Running      []statusEntry `json:"running"`
	Count        int           `json:"count"`
	TodaySeconds int64         `json:"today_seconds"`
	// Break es aditivo: null cuando no hay break activo.
	Break *breakOutput `json:"break"`
}
type createdEntry struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Project     string `json:"project"`
	Description string `json:"description"`
	StartedAt   int64  `json:"started_at"`
}
type resumedOutput struct {
	createdEntry
	CopyOf       int64  `json:"copy_of"`
	TaskUID      string `json:"task_uid"`
	TotalSeconds int64  `json:"total_seconds"`
}
type stoppedOutput struct {
	Stopped int `json:"stopped"`
}
type recentEntry struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Project     string `json:"project"`
	StartedAt   int64  `json:"started_at"`
	EndedAt     *int64 `json:"ended_at"`
	Seconds     int64  `json:"seconds"`
	UID         string `json:"uid"`
	Description string `json:"description"`
	TaskUID     string `json:"task_uid"`
}

// taskOutput resume una tarea: todas sus sesiones y el tiempo total.
type taskOutput struct {
	TaskUID        string `json:"task_uid"`
	Title          string `json:"title"`
	Project        string `json:"project"`
	Description    string `json:"description"`
	TotalSeconds   int64  `json:"total_seconds"`
	Running        bool   `json:"running"`
	RunningEntryID int64  `json:"running_entry_id"`
	LastEntryID    int64  `json:"last_entry_id"`
	LastActivity   int64  `json:"last_activity"`
	SessionCount   int    `json:"session_count"`
}
type listOutput struct {
	Running []statusEntry `json:"running"`
	Recent  []recentEntry `json:"recent"`
	// Tasks es aditivo: las mismas entradas agrupadas por tarea.
	Tasks []taskOutput `json:"tasks"`
}
type projectOutput struct {
	Name     string `json:"name"`
	LastUsed int64  `json:"last_used"`
	Seconds  int64  `json:"seconds"`
	// Campos aditivos: se omiten cuando están vacíos para no alterar el contrato existente.
	Client       string `json:"client,omitempty"`
	Organization string `json:"organization,omitempty"`
}
type errorOutput struct {
	Error string `json:"error"`
}

// entryOutput es la tarea completa que imprimen edit y trash --json.
type entryOutput struct {
	ID          int64  `json:"id"`
	UID         string `json:"uid"`
	Title       string `json:"title"`
	Project     string `json:"project"`
	Description string `json:"description"`
	StartedAt   int64  `json:"started_at"`
	EndedAt     *int64 `json:"ended_at"`
	DeletedAt   *int64 `json:"deleted_at,omitempty"`
}

// okOutput es la respuesta de las operaciones sin objeto que devolver, como eliminar.
type okOutput struct {
	OK bool `json:"ok"`
}

// orgOutput, clientOutput y projectObject son los objetos del catálogo en JSON.
type orgOutput struct {
	ID       int64  `json:"id"`
	UID      string `json:"uid"`
	Name     string `json:"name"`
	Archived bool   `json:"archived"`
}
type clientOutput struct {
	ID             int64  `json:"id"`
	UID            string `json:"uid"`
	Name           string `json:"name"`
	Archived       bool   `json:"archived"`
	OrganizationID *int64 `json:"organization_id"`
}
type projectObject struct {
	ID       int64  `json:"id"`
	UID      string `json:"uid"`
	Name     string `json:"name"`
	Archived bool   `json:"archived"`
	ClientID *int64 `json:"client_id"`
}

// Árbol del catálogo; un Organization o Client nulo es el grupo "sin organización" o "sin cliente".
type treeOrgOutput struct {
	Organization *orgOutput         `json:"organization"`
	Clients      []treeClientOutput `json:"clients"`
}
type treeClientOutput struct {
	Client   *clientOutput   `json:"client"`
	Projects []projectObject `json:"projects"`
}

func presentEntry(entry tracking.Entry) entryOutput {
	return entryOutput{entry.ID, entry.UID, entry.Title, entry.Project, entry.Description, entry.StartedAt, entry.EndedAt, entry.DeletedAt}
}

func presentStatus(entry tracking.Entry, now time.Time) statusEntry {
	return statusEntry{ID: entry.ID, Title: entry.Title, Project: entry.Project, StartedAt: entry.StartedAt, Elapsed: elapsed(now.Unix() - entry.StartedAt), UID: entry.UID, Description: entry.Description, TaskUID: entry.TaskUID}
}

func presentOrg(o catalog.Organization) orgOutput {
	return orgOutput{o.ID, o.UID, o.Name, o.ArchivedAt != nil}
}
func presentClient(c catalog.Client) clientOutput {
	return clientOutput{c.ID, c.UID, c.Name, c.ArchivedAt != nil, c.OrganizationID}
}
func presentProject(p catalog.Project) projectObject {
	return projectObject{p.ID, p.UID, p.Name, p.ArchivedAt != nil, p.ClientID}
}
