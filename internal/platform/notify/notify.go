// Package notify envía notificaciones de escritorio con acciones mediante notify-send.
package notify

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Action es un botón de la notificación; ID es lo que se devuelve al elegirlo.
type Action struct{ ID, Label string }

// Notification describe un aviso. Urgency es low, normal o critical.
type Notification struct {
	Title, Body string
	Actions     []Action
	Urgency     string
}

// args construye el argv de notify-send (sin shell).
func args(n Notification) []string {
	urgency := n.Urgency
	if urgency == "" {
		urgency = "normal"
	}
	argv := []string{"-a", "Nexus", "-u", urgency}
	for _, a := range n.Actions {
		argv = append(argv, "-A", a.ID+"="+a.Label)
	}
	return append(argv, n.Title, n.Body)
}

// Send muestra la notificación y espera la respuesta: devuelve el id de la acción elegida,
// o una cadena vacía si se descartó o expiró. Cancelar ctx cierra la espera.
func Send(ctx context.Context, n Notification) (string, error) {
	cmd := exec.CommandContext(ctx, "notify-send", args(n)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("notify-send: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}
