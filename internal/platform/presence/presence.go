package presence

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"time"
)

type Runner interface {
	Output(context.Context, string, ...string) ([]byte, error)
}
type execRunner struct{}

func (execRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

type State struct {
	Locked bool
	Idle   bool
	Source string
}

type Detector struct {
	runner  Runner
	timeout time.Duration
}

func New(r Runner) *Detector {
	if r == nil {
		r = execRunner{}
	}
	return &Detector{runner: r, timeout: 2 * time.Second}
}

func (d *Detector) output(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	return d.runner.Output(ctx, name, args...)
}

func (d *Detector) Activity(ctx context.Context) (State, error) {
	b, idleErr := d.output(ctx, "omarchy-shell", "idle", "status")
	var idle struct {
		Enabled     *bool `json:"enabled"`
		Idle        *bool `json:"idle"`
		InIdleCycle *bool `json:"inIdleCycle"`
	}
	if idleErr == nil {
		idleErr = json.Unmarshal(b, &idle)
		if idleErr == nil && (idle.Enabled == nil || idle.Idle == nil || idle.InIdleCycle == nil) {
			idleErr = errors.New("estado de inactividad incompleto")
		}
	}
	lockedBytes, lockedErr := d.output(ctx, "omarchy-shell", "lock", "isLocked")
	if idleErr == nil && lockedErr == nil {
		locked := strings.TrimSpace(string(lockedBytes))
		if locked == "true" || locked == "false" {
			return State{Locked: locked == "true", Idle: *idle.Idle || *idle.InIdleCycle, Source: "omarchy"}, nil
		}
	}
	return d.fallbackActivity(ctx)
}

func (d *Detector) fallbackActivity(ctx context.Context) (State, error) {
	state := State{Source: "fallback"}
	if _, err := d.output(ctx, "/usr/share/omarchy/bin/omarchy-hyprland-session-locked"); err == nil {
		state.Locked = true
	} else if exit, ok := err.(interface{ ExitCode() int }); ok && exit.ExitCode() == 1 {
		state.Locked = false
	} else {
		return State{}, err
	}
	if _, err := d.output(ctx, "pgrep", "-f", "[o]rg.omarchy.screensaver"); err == nil {
		state.Idle = true
	} else if _, err := d.output(ctx, "pgrep", "-x", "ttfx"); err == nil {
		state.Idle = true
	}
	return state, nil
}

func (d *Detector) Busy(ctx context.Context) (bool, string) {
	if b, e := d.output(ctx, "pactl", "list", "source-outputs", "short"); e == nil && len(strings.TrimSpace(string(b))) > 0 {
		return true, "micrófono"
	}
	if b, e := d.output(ctx, "hyprctl", "activewindow", "-j"); e == nil {
		var w struct {
			Fullscreen int    `json:"fullscreen"`
			Title      string `json:"title"`
			Class      string `json:"class"`
		}
		if json.Unmarshal(b, &w) == nil {
			if w.Fullscreen > 0 {
				return true, "pantalla completa"
			}
			text := strings.ToLower(w.Title + " " + w.Class)
			for _, term := range []string{"meet", "zoom", "teams", "webex", "discord", "jitsi", "slack"} {
				if strings.Contains(text, term) {
					return true, "reunión"
				}
			}
		}
	}
	return false, ""
}
