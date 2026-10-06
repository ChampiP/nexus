package wellbeing

import (
	"strconv"
	"time"
)

const inactiveReset = 2 * time.Minute

type Service struct {
	repo     Repository
	activity Activity
	clock    Clock
}

func NewService(repo Repository, activity Activity, clock Clock) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, activity: activity, clock: clock}
}

func (s *Service) values() map[string]string {
	v, err := s.repo.Load()
	if err != nil || v == nil {
		return map[string]string{}
	}
	return v
}
func (s *Service) settings(v map[string]string) Settings {
	x := DefaultSettings()
	if v["enabled"] != "" {
		x.Enabled = v["enabled"] != "false"
	}
	if n, e := strconv.ParseInt(v["every"], 10, 64); e == nil && n > 0 {
		x.Every = time.Duration(n) * time.Second
	}
	if n, e := strconv.ParseInt(v["duration"], 10, 64); e == nil && n > 0 {
		x.Duration = time.Duration(n) * time.Second
	}
	return x
}
func (s *Service) Settings() (Settings, error) {
	v, e := s.repo.Load()
	if e != nil {
		return Settings{}, e
	}
	return s.settings(v), nil
}
func valid(x Settings) bool {
	if x.Every == 0 {
		x.Every = 30 * time.Minute
	}
	if x.Duration == 0 {
		x.Duration = 30 * time.Second
	}
	validEvery := false
	for _, d := range []time.Duration{10, 20, 30, 45, 60} {
		if x.Every == time.Duration(d)*time.Minute {
			validEvery = true
		}
	}
	validDuration := false
	for _, d := range []time.Duration{10 * time.Second, 15 * time.Second, 20 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute} {
		if x.Duration == d {
			validDuration = true
		}
	}
	return validEvery && validDuration
}
func (s *Service) UpdateSettings(x Settings) error {
	if x.Every == 0 {
		x.Every = 30 * time.Minute
	}
	if x.Duration == 0 {
		x.Duration = 30 * time.Second
	}
	if !valid(x) {
		return ErrInvalidSetting
	}
	return s.repo.Save(map[string]string{
		"enabled":  strconv.FormatBool(x.Enabled),
		"every":    strconv.FormatInt(int64(x.Every/time.Second), 10),
		"duration": strconv.FormatInt(int64(x.Duration/time.Second), 10),
	})
}

// Due mide continuidad activa frente al equipo; dos minutos de observaciones inactivas reinician el tramo.
func (s *Service) Due(now time.Time) (Reminder, bool) {
	v := s.values()
	active, err := s.activity.Active(now)
	if err != nil {
		return Reminder{}, false
	}
	if !active {
		if parseTime(v["inactive_since"]).IsZero() {
			v["inactive_since"] = strconv.FormatInt(now.Unix(), 10)
			_ = s.repo.Save(v)
		}
		return Reminder{}, false
	}
	start := parseTime(v["active_since"])
	inactiveSince := parseTime(v["inactive_since"])
	if !inactiveSince.IsZero() {
		if now.Sub(inactiveSince) >= inactiveReset || start.IsZero() {
			start = now
		}
		delete(v, "inactive_since")
		// Save solo hace UPSERT de las claves presentes; la baja se persiste aparte.
		if err := s.deleteKey("inactive_since"); err != nil {
			return Reminder{}, false
		}
	}
	if start.IsZero() {
		start = now
	}
	v["active_since"] = strconv.FormatInt(start.Unix(), 10)
	if err := s.repo.Save(v); err != nil {
		return Reminder{}, false
	}
	breakActive, err := s.activity.BreakActive()
	if err != nil || breakActive {
		return Reminder{}, false
	}
	cfg := s.settings(v)
	if !cfg.Enabled {
		return Reminder{}, false
	}
	if until := parseTime(v["snoozed_until"]); until.After(now) {
		return Reminder{}, false
	}
	if until := parseTime(v["dnd_until"]); until.After(now) {
		return Reminder{}, false
	}
	last := parseTime(v["last_shown_at"])
	if last.After(start) {
		start = last
	}
	if now.Sub(start) < cfg.Every {
		return Reminder{}, false
	}
	i := parseInt(v["rotation"])
	return Reminder{ID: int64(i + 1), Duration: cfg.Duration, Message: messages[i%len(messages)], Tip: tips[i%len(tips)]}, true
}
func optionalTime(x time.Time) *time.Time {
	if x.IsZero() {
		return nil
	}
	return &x
}
func parseTime(x string) time.Time {
	n, _ := strconv.ParseInt(x, 10, 64)
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(n, 0)
}
func parseInt(x string) int {
	n, _ := strconv.Atoi(x)
	if n < 0 {
		return 0
	}
	return n
}
func (s *Service) MarkShown(now time.Time) error {
	v := s.values()
	v["last_shown_at"] = strconv.FormatInt(now.Unix(), 10)
	v["rotation"] = strconv.Itoa(parseInt(v["rotation"]) + 1)
	return s.saveEvent(map[string]string{"last_shown_at": v["last_shown_at"], "rotation": v["rotation"]}, "shown", now)
}
func (s *Service) Done(now time.Time) error { return s.record(now, "done") }
func (s *Service) Skip(now time.Time) error { return s.record(now, "skipped") }
func (s *Service) record(now time.Time, event string) error {
	return s.saveEvent(nil, event, now)
}
func (s *Service) saveEvent(values map[string]string, event string, now time.Time) error {
	if repo, ok := s.repo.(eventRepository); ok {
		if values == nil {
			values = map[string]string{}
		}
		return repo.SaveEvent(values, event, now.Unix())
	}
	if values == nil {
		values = s.values()
	}
	if event == "shown" || event == "done" || event == "skipped" {
		key := event + ":" + now.Format("2006-01-02")
		values[key] = strconv.Itoa(parseInt(values[key]) + 1)
	}
	return s.repo.Save(values)
}

// ResultRecorder es lo que ApplyResult necesita para registrar la respuesta a una pausa.
type ResultRecorder interface {
	Done(time.Time) error
	Skip(time.Time) error
	Snooze(time.Time, time.Duration) error
}

// ApplyResult traduce el resultado de un presentador en un registro. Solo "done", "snooze" y
// "skip" registran algo; "delegated" (la capa nativa informa por su cuenta), "default"
// (clic en la notificación), "" (cerrada o expirada) y cualquier otro valor no cuentan.
func ApplyResult(r ResultRecorder, now time.Time, result string) error {
	switch result {
	case "done":
		return r.Done(now)
	case "snooze":
		return r.Snooze(now, 10*time.Minute)
	case "skip":
		return r.Skip(now)
	}
	return nil
}

func (s *Service) Snooze(now time.Time, d time.Duration) error {
	return s.saveEvent(map[string]string{"snoozed_until": strconv.FormatInt(now.Add(d).Unix(), 10)}, "snoozed", now)
}
func (s *Service) DND(now time.Time, d time.Duration) error {
	return s.repo.Save(map[string]string{"dnd_until": strconv.FormatInt(now.Add(d).Unix(), 10)})
}
func (s *Service) ClearDND() error { return s.deleteKey("dnd_until") }

// deleteKey usa Delete si el repositorio lo ofrece; si no, reemplaza el mapa completo.
func (s *Service) deleteKey(key string) error {
	if deleter, ok := s.repo.(interface{ Delete(string) error }); ok {
		return deleter.Delete(key)
	}
	v := s.values()
	delete(v, key)
	return s.repo.Save(v)
}
func (s *Service) Status(now time.Time) (Status, error) {
	v, e := s.repo.Load()
	if e != nil {
		return Status{}, e
	}
	cfg := s.settings(v)
	active, err := s.activity.Active(now)
	if err != nil {
		return Status{}, err
	}
	start := parseTime(v["active_since"])
	last := parseTime(v["last_shown_at"])
	if last.After(start) {
		start = last
	}
	next := start.Add(cfg.Every)
	if start.IsZero() {
		next = now.Add(cfg.Every)
	}
	activeFor := time.Duration(0)
	if active && !parseTime(v["active_since"]).IsZero() {
		activeFor = now.Sub(parseTime(v["active_since"]))
		if activeFor < 0 {
			activeFor = 0
		}
	}
	startDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var counters Counters
	if repo, ok := s.repo.(eventRepository); ok {
		counters, err = repo.Counters(startDay.Unix(), startDay.AddDate(0, 0, 1).Unix())
		if err != nil {
			return Status{}, err
		}
	} else {
		day := now.Format("2006-01-02")
		counters = Counters{Shown: parseInt(v["shown:"+day]), Done: parseInt(v["done:"+day]), Skipped: parseInt(v["skipped:"+day]), Snoozed: parseInt(v["snoozed:"+day])}
	}
	return Status{Settings: cfg, NextDue: next, Active: active, ActiveFor: activeFor, SnoozedUntil: optionalTime(parseTime(v["snoozed_until"])), DNDUntil: optionalTime(parseTime(v["dnd_until"])), Counters: counters}, nil
}
