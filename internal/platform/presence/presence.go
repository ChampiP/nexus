package presence

import (
	"context"
	"encoding/json"
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
func (d *Detector) Busy(ctx context.Context) (bool, string) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	if b, e := d.runner.Output(ctx, "pactl", "list", "source-outputs", "short"); e == nil && len(strings.TrimSpace(string(b))) > 0 {
		return true, "micrófono"
	}
	if b, e := d.runner.Output(ctx, "hyprctl", "activewindow", "-j"); e == nil {
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
