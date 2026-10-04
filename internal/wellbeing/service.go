package wellbeing

import (
	"strconv"
	"time"
)

const idleReset = 5 * time.Minute

type Service struct {
	repo  Repository
	work  Work
	clock Clock
}

func NewService(repo Repository, work Work, clock Clock) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, work: work, clock: clock}
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
	v := s.values()
	v["enabled"] = strconv.FormatBool(x.Enabled)
	v["every"] = strconv.FormatInt(int64(x.Every/time.Second), 10)
	v["duration"] = strconv.FormatInt(int64(x.Duration/time.Second), 10)
	return s.repo.Save(v)
}
func (s *Service) Due(now time.Time) (Reminder, bool) {
	v := s.values()
	running, err := s.work.WorkRunning()
	if err != nil {
		return Reminder{}, false
	}
	if !running {
		if since := parseTime(v["stopped_since"]); since.IsZero() {
			v["stopped_since"] = strconv.FormatInt(now.Unix(), 10)
		} else if now.Sub(since) > idleReset {
			delete(v, "work_since")
			delete(v, "last_shown_at")
		}
		_ = s.repo.Save(v)
		return Reminder{}, false
	}
	if stopped := parseTime(v["stopped_since"]); !stopped.IsZero() {
		delete(v, "stopped_since")
		if now.Sub(stopped) > idleReset {
			v["work_since"] = strconv.FormatInt(now.Unix(), 10)
		}
		_ = s.repo.Save(v)
	}
	breakActive, err := s.work.BreakActive()
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
	start := parseTime(v["work_since"])
	if start.IsZero() {
		start = now
		v["work_since"] = strconv.FormatInt(now.Unix(), 10)
		_ = s.repo.Save(v)
	}
	last := parseTime(v["last_shown_at"])
	if last.After(start) {
		start = last
	}
	if now.Sub(start) < cfg.Every {
		return Reminder{}, false
	}
	i := parseInt(v["rotation"])
	r := Reminder{ID: int64(i + 1), Duration: cfg.Duration, Message: messages[i%len(messages)], Tip: tips[i%len(tips)]}
	return r, true
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
	day := now.Format("2006-01-02")
	v["shown:"+day] = strconv.Itoa(parseInt(v["shown:"+day]) + 1)
	return s.repo.Save(v)
}
func (s *Service) Done(now time.Time) error { return s.count(now, "done") }
func (s *Service) Skip(now time.Time) error { return s.count(now, "skipped") }
func (s *Service) count(now time.Time, key string) error {
	v := s.values()
	day := now.Format("2006-01-02")
	v[key+":"+day] = strconv.Itoa(parseInt(v[key+":"+day]) + 1)
	return s.repo.Save(v)
}
func (s *Service) Snooze(now time.Time, d time.Duration) error {
	v := s.values()
	v["snoozed_until"] = strconv.FormatInt(now.Add(d).Unix(), 10)
	return s.repo.Save(v)
}
func (s *Service) DND(now time.Time, d time.Duration) error {
	v := s.values()
	v["dnd_until"] = strconv.FormatInt(now.Add(d).Unix(), 10)
	return s.repo.Save(v)
}
func (s *Service) ClearDND() error { v := s.values(); delete(v, "dnd_until"); return s.repo.Save(v) }
func (s *Service) Status(now time.Time) (Status, error) {
	v, e := s.repo.Load()
	if e != nil {
		return Status{}, e
	}
	cfg := s.settings(v)
	start := parseTime(v["work_since"])
	last := parseTime(v["last_shown_at"])
	if last.After(start) {
		start = last
	}
	next := start.Add(cfg.Every)
	if start.IsZero() {
		next = now.Add(cfg.Every)
	}
	day := now.Format("2006-01-02")
	return Status{Settings: cfg, NextDue: next, SnoozedUntil: optionalTime(parseTime(v["snoozed_until"])), DNDUntil: optionalTime(parseTime(v["dnd_until"])), Counters: Counters{parseInt(v["shown:"+day]), parseInt(v["done:"+day]), parseInt(v["skipped:"+day])}}, nil
}
