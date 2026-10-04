package presence

import (
	"context"
	"strings"
	"testing"
)

type fixture map[string]string

func (f fixture) Output(_ context.Context, n string, _ ...string) ([]byte, error) {
	x, ok := f[n]
	if !ok {
		return nil, context.Canceled
	}
	return []byte(x), nil
}
func TestBusyParsing(t *testing.T) {
	for _, tc := range []struct {
		p    fixture
		busy bool
	}{{fixture{"pactl": "42\tapp\tmodule"}, true}, {fixture{"hyprctl": `{"fullscreen":2,"title":"editor","class":"x"}`}, true}, {fixture{"hyprctl": `{"fullscreen":0,"title":"Daily Teams call","class":"browser"}`}, true}, {fixture{"pactl": "", "hyprctl": `{"fullscreen":0,"title":"editor","class":"code"}`}, false}} {
		b, _ := New(tc.p).Busy(context.Background())
		if b != tc.busy {
			t.Errorf("busy=%v expected %v fixture=%v", b, tc.busy, strings.TrimSpace(tc.p["hyprctl"]))
		}
	}
}
