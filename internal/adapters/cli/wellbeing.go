package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"nexus/internal/wellbeing"
)

type Wellbeing interface {
	Settings() (wellbeing.Settings, error)
	UpdateSettings(wellbeing.Settings) error
	Due(time.Time) (wellbeing.Reminder, bool)
	MarkShown(time.Time) error
	Done(time.Time) error
	Skip(time.Time) error
	Snooze(time.Time, time.Duration) error
	DND(time.Time, time.Duration) error
	ClearDND() error
	Status(time.Time) (wellbeing.Status, error)
}
type ReminderPresenter interface {
	Show(context.Context, wellbeing.Reminder) (string, error)
}

func runWellbeing(args []string, svc Wellbeing, p PresenterBridge, now time.Time, out io.Writer) error {
	if svc == nil {
		return fmt.Errorf("las pausas activas no están disponibles")
	}
	if len(args) == 0 {
		return showWellbeingStatus(svc, now, false, out)
	}
	sub := args[0]
	rest := args[1:]
	if sub == "estado" {
		if len(rest) > 1 || (len(rest) == 1 && rest[0] != "--json") {
			return fmt.Errorf("uso: nexus pausa estado [--json]")
		}
		return showWellbeingStatus(svc, now, len(rest) == 1, out)
	}
	if sub == "activar" || sub == "desactivar" {
		if len(rest) != 0 {
			return fmt.Errorf("uso: nexus pausa %s", sub)
		}
		cfg, e := svc.Settings()
		if e != nil {
			return e
		}
		cfg.Enabled = sub == "activar"
		if e = svc.UpdateSettings(cfg); e != nil {
			return e
		}
		label := "desactivadas"
		if cfg.Enabled {
			label = "activadas"
		}
		_, e = fmt.Fprintf(out, "Pausas activas %s\n", label)
		return e
	}
	if sub == "reanudar" {
		if len(rest) > 0 {
			return fmt.Errorf("uso: nexus pausa reanudar")
		}
		return svc.ClearDND()
	}
	if sub == "no-molestar" {
		mins := 60
		if len(rest) > 1 {
			return fmt.Errorf("uso: nexus pausa no-molestar [minutos]")
		}
		if len(rest) == 1 {
			n, e := strconv.Atoi(rest[0])
			if e != nil || n <= 0 {
				return fmt.Errorf("los minutos deben ser un entero positivo")
			}
			mins = n
		}
		if e := svc.DND(now, time.Duration(mins)*time.Minute); e != nil {
			return e
		}
		_, e := fmt.Fprintf(out, "No molestar hasta las %s\n", now.Add(time.Duration(mins)*time.Minute).Format("15:04"))
		return e
	}
	if sub == "hecho" || sub == "posponer" || sub == "saltar" {
		if len(rest) > 0 {
			return fmt.Errorf("uso: nexus pausa %s", sub)
		}
		switch sub {
		case "hecho":
			return svc.Done(now)
		case "posponer":
			return svc.Snooze(now, 10*time.Minute)
		default:
			return svc.Skip(now)
		}
	}
	if sub == "ahora" {
		r, ok := svc.Due(now)
		if !ok {
			cfg, e := svc.Settings()
			if e != nil {
				return e
			}
			r = wellbeing.Reminder{Duration: cfg.Duration, Message: "Es momento de moverte un poco.", Tip: "Ponte de pie y estira la espalda."}
		}
		if e := svc.MarkShown(now); e != nil {
			return e
		}
		if p == nil {
			return fmt.Errorf("no hay presentador disponible")
		}
		result, e := p.Show(context.Background(), r)
		if e != nil {
			return e
		}
		return wellbeing.ApplyResult(svc, now, result)

	}
	if len(args) == 0 {
		return nil
	}
	return updateWellbeingSettings(args, svc, now, out)
}

type PresenterBridge interface {
	Show(context.Context, wellbeing.Reminder) (string, error)
}

func updateWellbeingSettings(args []string, s Wellbeing, now time.Time, out io.Writer) error {
	cfg, e := s.Settings()
	if e != nil {
		return e
	}
	changed := false
	for i := 0; i < len(args); i++ {
		if i+1 >= len(args) {
			return fmt.Errorf("%s requiere un valor", args[i])
		}
		value := args[i+1]
		switch args[i] {
		case "--cada":
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("--cada acepta 10, 20, 30, 45 o 60 minutos")
			}
			cfg.Every = time.Duration(n) * time.Minute
		case "--dura":
			d, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("--dura acepta 10s, 15s, 20s, 30s, 1m, 2m o 5m")
			}
			cfg.Duration = d
		default:
			return fmt.Errorf("opción desconocida %q", args[i])
		}
		i++
		changed = true
	}
	if !changed {
		return fmt.Errorf("uso: nexus pausa [estado|activar|desactivar|no-molestar|reanudar|ahora]")
	}
	if e = s.UpdateSettings(cfg); e != nil {
		return e
	}
	return showWellbeingStatus(s, now, false, out)
}
func showWellbeingStatus(s Wellbeing, now time.Time, jsonMode bool, out io.Writer) error {
	st, e := s.Status(now)
	if e != nil {
		return e
	}
	if jsonMode {
		return json.NewEncoder(out).Encode(st)
	}
	text := "Pausas activas: desactivadas"
	if st.DNDUntil != nil && st.DNDUntil.After(now) {
		text = fmt.Sprintf("Pausas activas: en no molestar hasta las %s", st.DNDUntil.Format("15:04"))
	} else if st.Settings.Enabled {
		text = fmt.Sprintf("Pausas activas: cada %d min, %s", int(st.Settings.Every/time.Minute), shortDuration(st.Settings.Duration))
		if !st.Active {
			text += " · inactivo"
		} else if st.ActiveFor < time.Minute {
			text += fmt.Sprintf(" · recién empiezas · próxima en %s", shortMinutes(st.NextDue.Sub(now)))
		} else {
			text += fmt.Sprintf(" · llevas %s frente a la pantalla · próxima en %s", shortMinutes(st.ActiveFor), shortMinutes(st.NextDue.Sub(now)))
		}
	}
	text += fmt.Sprintf(" · hoy: %d mostradas, %d hechas, %d saltadas, %d pospuestas", st.Counters.Shown, st.Counters.Done, st.Counters.Skipped, st.Counters.Snoozed)
	_, e = io.WriteString(out, text+"\n")
	return e
}
func shortMinutes(d time.Duration) string {
	minutes := int(d / time.Minute)
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("%d min", minutes)
}

func shortDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d s", int(d/time.Second))
	}
	return strings.TrimSuffix(d.String(), "0s")
}
