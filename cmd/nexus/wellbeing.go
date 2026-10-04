package main

import (
	"context"
	"encoding/json"
	"nexus/internal/adapters/daemon"
	"os/exec"
	"strings"
	"time"

	"nexus/internal/platform/notify"
	"nexus/internal/wellbeing"
)

type wellbeingPresenter struct{}

func (wellbeingPresenter) Show(ctx context.Context, r wellbeing.Reminder) (string, error) {
	if summonWellbeing(ctx, r) {
		return daemon.ResultDelegated, nil
	}
	return notify.Send(ctx, notify.Notification{Title: "🚶 Hora de moverte", Body: r.Message + " " + r.Tip, Urgency: "critical", Actions: []notify.Action{{ID: "default", Label: "Abrir Nexus"}, {ID: "snooze", Label: "Posponer 10 min"}, {ID: "skip", Label: "Saltar"}}})
}

// summonWellbeing pide al shell la capa de pausa; solo cuenta como mostrada si el plugin existe
// y responde "ok" (el shell devuelve código 0 incluso con "unknown").
func summonWellbeing(ctx context.Context, r wellbeing.Reminder) bool {
	payload, err := json.Marshal(map[string]any{"message": r.Message, "tip": r.Tip, "seconds": int(r.Duration / time.Second)})
	if err != nil {
		return false
	}
	out, err := exec.CommandContext(ctx, "omarchy-shell", "shell", "summon", pausePluginID, string(payload)).Output()
	return err == nil && strings.TrimSpace(string(out)) == "ok"
}

// pausePluginID es el plugin de la capa de pausas; es distinto del widget para no abrir su popup.
const pausePluginID = "champip.nexus-pausa"
