package notify

import (
	"reflect"
	"testing"
)

func TestArgs(t *testing.T) {
	got := args(Notification{
		Title:   "Título",
		Body:    "Cuerpo; $(no) 'shell'",
		Urgency: "critical",
		Actions: []Action{{"back", "Volver al trabajo"}, {"extend", "+10 min"}},
	})
	want := []string{"-a", "Nexus", "-u", "critical", "-A", "back=Volver al trabajo", "-A", "extend=+10 min", "Título", "Cuerpo; $(no) 'shell'"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %q, want %q", got, want)
	}
}

func TestArgsDefaultUrgencyNoActions(t *testing.T) {
	got := args(Notification{Title: "a", Body: "b"})
	want := []string{"-a", "Nexus", "-u", "normal", "a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %q, want %q", got, want)
	}
}
