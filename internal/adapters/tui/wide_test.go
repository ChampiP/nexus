package tui

import "testing"

func TestWideLayout(t *testing.T) {
	tests := []struct {
		name                string
		width               int
		ok                  bool
		left, center, right int
	}{
		{name: "below threshold", width: 119},
		{name: "threshold", width: 120, ok: true, left: 40, center: 42, right: 36},
		{name: "wide terminal", width: 160, ok: true, left: 53, center: 56, right: 49},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, left, center, right := wideLayout(tt.width)
			if ok != tt.ok || left != tt.left || center != tt.center || right != tt.right {
				t.Fatalf("wideLayout(%d) = (%t, %d, %d, %d), want (%t, %d, %d, %d)", tt.width, ok, left, center, right, tt.ok, tt.left, tt.center, tt.right)
			}
			if ok && (left < 36 || center < 36 || right < 36) {
				t.Fatalf("wideLayout(%d) produced a column narrower than 36: %d, %d, %d", tt.width, left, center, right)
			}
		})
	}
}
