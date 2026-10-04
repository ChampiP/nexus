package presence

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type call struct {
	name string
	args []string
}
type exitFailure int

func (e exitFailure) Error() string { return "command exited" }
func (e exitFailure) ExitCode() int { return int(e) }

type fixture struct {
	outputs map[string]string
	errors  map[string]error
	calls   []call
}

func (f *fixture) Output(_ context.Context, n string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, call{n, append([]string(nil), args...)})
	key := strings.TrimSpace(n + " " + strings.Join(args, " "))
	if err := f.errors[key]; err != nil {
		return nil, err
	}
	x, ok := f.outputs[key]
	if !ok {
		return nil, errors.New("unexpected command")
	}
	return []byte(x), nil
}

func TestBusyParsing(t *testing.T) {
	for _, tc := range []struct {
		p    map[string]string
		busy bool
	}{
		{map[string]string{"pactl list source-outputs short": "42\\tapp\\tmodule"}, true},
		{map[string]string{"hyprctl activewindow -j": `{"fullscreen":2,"title":"editor","class":"x"}`}, true},
		{map[string]string{"hyprctl activewindow -j": `{"fullscreen":0,"title":"Daily Teams call","class":"browser"}`}, true},
		{map[string]string{"hyprctl activewindow -j": `{"fullscreen":0,"title":"editor","class":"code"}`}, false},
	} {
		f := &fixture{outputs: tc.p}
		b, _ := New(f).Busy(context.Background())
		if b != tc.busy {
			t.Errorf("busy=%v expected %v", b, tc.busy)
		}
	}
}

func TestActivityParsesIdleAndLockState(t *testing.T) {
	cases := []struct {
		name, idle, locked string
		want               State
	}{
		{"active", `{"enabled":true,"idle":false,"inIdleCycle":false}`, "false", State{Source: "omarchy"}},
		{"idle", `{"enabled":true,"idle":true,"inIdleCycle":true}`, "false", State{Idle: true, Source: "omarchy"}},
		{"idle cycle", `{"enabled":true,"idle":false,"inIdleCycle":true}`, "false", State{Idle: true, Source: "omarchy"}},
		{"locked", `{"enabled":true,"idle":false,"inIdleCycle":false}`, "true", State{Locked: true, Source: "omarchy"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fixture{outputs: map[string]string{"omarchy-shell idle status": tc.idle, "omarchy-shell lock isLocked": tc.locked}}
			got, err := New(f).Activity(context.Background())
			if err != nil || got != tc.want {
				t.Fatalf("Activity() = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}

func TestActivityMalformedJSONUsesFallback(t *testing.T) {
	f := &fixture{
		outputs: map[string]string{"omarchy-shell idle status": "{bad", "/usr/share/omarchy/bin/omarchy-hyprland-session-locked": ""},
		errors:  map[string]error{"omarchy-shell lock isLocked": errors.New("unavailable"), "pgrep -f [o]rg.omarchy.screensaver": errors.New("not found"), "pgrep -x ttfx": errors.New("not found")},
	}
	got, err := New(f).Activity(context.Background())
	if err != nil || got != (State{Locked: true, Source: "fallback"}) {
		t.Fatalf("Activity() = %+v, %v", got, err)
	}
	want := []call{{"omarchy-shell", []string{"idle", "status"}}, {"omarchy-shell", []string{"lock", "isLocked"}}, {"/usr/share/omarchy/bin/omarchy-hyprland-session-locked", nil}, {"pgrep", []string{"-f", "[o]rg.omarchy.screensaver"}}, {"pgrep", []string{"-x", "ttfx"}}}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("commands = %#v, want %#v", f.calls, want)
	}
}

func TestActivityFallbackReportsLockAndScreensaver(t *testing.T) {
	cases := []struct {
		name   string
		locked string
		screen string
		want   State
	}{
		{"locked fallback", "exit", "missing", State{Locked: true, Source: "fallback"}},
		{"screensaver", "unlocked", "present", State{Idle: true, Source: "fallback"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fixture{outputs: map[string]string{}, errors: map[string]error{}}
			f.errors["omarchy-shell idle status"] = errors.New("unavailable")
			f.errors["omarchy-shell lock isLocked"] = errors.New("unavailable")
			if tc.locked == "exit" {
				f.errors["/usr/share/omarchy/bin/omarchy-hyprland-session-locked"] = nil
				f.outputs["/usr/share/omarchy/bin/omarchy-hyprland-session-locked"] = ""
			} else {
				f.errors["/usr/share/omarchy/bin/omarchy-hyprland-session-locked"] = exitFailure(1)
			}
			if tc.screen == "present" {
				f.outputs["pgrep -f [o]rg.omarchy.screensaver"] = "123"
			} else {
				f.errors["pgrep -f [o]rg.omarchy.screensaver"] = errors.New("not found")
			}
			f.errors["pgrep -x ttfx"] = errors.New("not found")
			got, err := New(f).Activity(context.Background())
			if err != nil || got != tc.want {
				t.Fatalf("Activity() = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}
