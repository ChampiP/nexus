package wellbeing

import (
	"errors"
	"time"
)

var ErrInvalidSetting = errors.New("invalid wellbeing setting")

type Settings struct {
	Enabled  bool          `json:"enabled"`
	Every    time.Duration `json:"every"`
	Duration time.Duration `json:"duration"`
}

type Reminder struct {
	ID       int64
	Duration time.Duration
	Message  string
	Tip      string
}

type Counters struct {
	Shown   int `json:"shown"`
	Done    int `json:"done"`
	Skipped int `json:"skipped"`
}

type Status struct {
	Settings     Settings   `json:"settings"`
	NextDue      time.Time  `json:"next_due"`
	SnoozedUntil *time.Time `json:"snoozed_until,omitempty"`
	DNDUntil     *time.Time `json:"dnd_until,omitempty"`
	Counters     Counters   `json:"today"`
}

func DefaultSettings() Settings {
	return Settings{Enabled: true, Every: 30 * time.Minute, Duration: 30 * time.Second}
}

var tips = []string{
	"Estira la espalda.", "Camina un poco por la casa.", "Mira por la ventana 20 segundos a lo lejos.",
	"Gira los hombros con suavidad.", "Toma un vaso de agua.", "Estira las piernas.",
	"Respira hondo varias veces.", "Relaja la mandíbula y las manos.", "Ponte de pie un momento.", "Descansa la vista.",
}
var messages = []string{
	"Caminar 5 minutos cada 30 baja el azúcar en sangre y la presión.",
	"Una pausa breve ayuda a reducir el tiempo sedentario.",
	"Cambiar de postura con frecuencia puede aliviar la tensión muscular.",
	"Mirar a lo lejos ayuda a descansar la vista.",
	"Unos minutos de movimiento activan la circulación.",
}
