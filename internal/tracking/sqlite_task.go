package tracking

import (
	"database/sql"
	"fmt"
	"sort"
)

// taskKey identifica la tarea de una fila; COALESCE cubre filas escritas por un binario anterior.
const taskKey = `COALESCE(task_uid, uid)`

// RecentTasks agrupa las sesiones de trabajo vivas por tarea, la de actividad más reciente primero.
func (s *SQLite) RecentTasks(now int64, limit int) ([]TaskSummary, error) {
	if limit <= 0 {
		return []TaskSummary{}, nil
	}
	entries, err := s.queryEntries(`SELECT ` + entryColumns + ` FROM entries WHERE deleted_at IS NULL AND kind = 'work' ORDER BY started_at, id`)
	if err != nil {
		return nil, err
	}
	byTask := map[string]*TaskSummary{}
	var order []*TaskSummary
	for _, e := range entries {
		task := byTask[e.TaskUID]
		if task == nil {
			task = &TaskSummary{TaskUID: e.TaskUID}
			byTask[e.TaskUID] = task
			order = append(order, task)
		}
		// Las filas llegan de la más antigua a la más reciente: la última define los datos visibles.
		task.Title, task.Project, task.ProjectID, task.Description = e.Title, e.Project, e.ProjectID, e.Description
		task.LastEntryID = e.ID
		task.SessionCount++
		end := now
		if e.EndedAt != nil {
			end = *e.EndedAt
		} else {
			task.Running, task.RunningEntryID = true, e.ID
		}
		task.TotalSeconds += max(0, end-e.StartedAt)
		task.LastActivity = max(task.LastActivity, end)
	}
	tasks := make([]TaskSummary, len(order))
	for i, task := range order {
		tasks[i] = *task
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].LastActivity != tasks[j].LastActivity {
			return tasks[i].LastActivity > tasks[j].LastActivity
		}
		return tasks[i].LastEntryID > tasks[j].LastEntryID
	})
	if len(tasks) > limit {
		tasks = tasks[:limit]
	}
	return tasks, nil
}

// TaskTotal suma los segundos de las sesiones vivas de la tarea.
func (s *SQLite) TaskTotal(taskUID string, now int64) (int64, error) {
	var total int64
	err := s.db.QueryRow(`SELECT COALESCE(SUM(MAX(0, COALESCE(ended_at, ?) - started_at)), 0) FROM entries WHERE deleted_at IS NULL AND `+taskKey+` = ?`, now, taskUID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("query task total: %w", err)
	}
	return total, nil
}

// RunningInTask devuelve la sesión en curso de la tarea.
func (s *SQLite) RunningInTask(taskUID string) (Entry, bool, error) {
	entries, err := s.queryEntries(`SELECT `+entryColumns+` FROM entries WHERE deleted_at IS NULL AND ended_at IS NULL AND `+taskKey+` = ? ORDER BY started_at, id LIMIT 1`, taskUID)
	if err != nil || len(entries) == 0 {
		return Entry{}, false, err
	}
	return entries[0], true, nil
}

// UpdateTask reescribe los campos editables de todas las sesiones vivas de la tarea.
func (s *SQLite) UpdateTask(taskUID string, entry Entry) error {
	result, err := s.db.Exec(`UPDATE entries SET title = ?, description = ?, project = ?, project_id = NULLIF(?, 0) WHERE deleted_at IS NULL AND `+taskKey+` = ?`, entry.Title, entry.Description, entry.Project, entry.ProjectID, taskUID)
	return expectRow(result, err, "update task")
}

// SoftDeleteTask elimina todas las sesiones vivas de la tarea en el mismo instante.
func (s *SQLite) SoftDeleteTask(taskUID string, at int64) error {
	result, err := s.db.Exec(`UPDATE entries SET
		deleted_running = CASE WHEN ended_at IS NULL THEN 1 ELSE 0 END,
		ended_at = COALESCE(ended_at, ?), deleted_at = ?
		WHERE deleted_at IS NULL AND `+taskKey+` = ?`, at, at, taskUID)
	return expectRow(result, err, "delete task")
}

// RestoreTask restaura las sesiones borradas en el último borrado de la tarea (mismo deleted_at).
func (s *SQLite) RestoreTask(taskUID string, resumeSince int64) error {
	var at sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(deleted_at) FROM entries WHERE deleted_at IS NOT NULL AND `+taskKey+` = ?`, taskUID).Scan(&at); err != nil {
		return fmt.Errorf("restore task: %w", err)
	}
	if !at.Valid {
		return ErrNotFound
	}
	result, err := s.db.Exec(`UPDATE entries SET
		ended_at = CASE WHEN id = (
			SELECT candidate.id FROM entries AS candidate
			WHERE candidate.deleted_at = ? AND COALESCE(candidate.task_uid, candidate.uid) = ? AND candidate.deleted_at >= ?
			  AND (candidate.deleted_running = 1 OR
				(candidate.deleted_running IS NULL AND candidate.ended_at = candidate.deleted_at))
			ORDER BY CASE WHEN candidate.deleted_running = 1 THEN 0 ELSE 1 END,
				candidate.started_at DESC, candidate.id DESC LIMIT 1
		) AND NOT EXISTS (
			SELECT 1 FROM entries AS active
			WHERE COALESCE(active.task_uid, active.uid) = COALESCE(entries.task_uid, entries.uid)
			  AND active.deleted_at IS NULL AND active.ended_at IS NULL
		) THEN NULL ELSE ended_at END,
		deleted_at = NULL,
		deleted_running = NULL
		WHERE deleted_at = ? AND `+taskKey+` = ?`, at.Int64, taskUID, resumeSince, at.Int64, taskUID)
	return expectRow(result, err, "restore task")
}
