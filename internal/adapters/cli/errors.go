package cli

import (
	"errors"

	"nexus/internal/catalog"
	"nexus/internal/countdown"
	"nexus/internal/tracking"
	"nexus/internal/wellbeing"
)

// localize translates domain sentinel errors into user-facing Spanish; the domain keeps neutral text.
func localize(err error) error {
	switch {
	case errors.Is(err, tracking.ErrEmptyTitle):
		return errors.New("el título no puede estar vacío")
	case errors.Is(err, tracking.ErrNotRunning):
		return errors.New("el temporizador no está en curso")
	case errors.Is(err, tracking.ErrNotFound):
		return errors.New("la tarea no existe")
	case errors.Is(err, countdown.ErrBreakActive):
		return errors.New("Ya hay un break en curso")
	case errors.Is(err, countdown.ErrNoActiveBreak):
		return errors.New("No hay un break activo")
	case errors.Is(err, countdown.ErrInvalidDuration):
		return errors.New("La duración debe estar entre 1 minuto y 8 horas")
	case errors.Is(err, wellbeing.ErrInvalidSetting):
		return errors.New("configuración de pausa activa no válida")
	case errors.Is(err, catalog.ErrEmptyName):
		return errors.New("el nombre no puede estar vacío")
	case errors.Is(err, catalog.ErrMergeSelf):
		return errors.New("no se puede unir un proyecto consigo mismo")
	case errors.Is(err, catalog.ErrNotFound):
		return errors.New("no existe")
	case errors.Is(err, catalog.ErrDuplicateName):
		return errors.New("ya existe un elemento con ese nombre")
	default:
		return err
	}
}

// kind describe un tipo de elemento del catálogo con el texto necesario para redactar mensajes.
type kind struct {
	label     string // "organización"
	article   string // "la"
	feminine  bool
	duplicate string // mensaje completo para un nombre repetido
	missing   string // mensaje completo para un elemento inexistente
}

var (
	kindOrg     = kind{"organización", "la", true, "ya existe una organización con ese nombre", "la organización no existe"}
	kindClient  = kind{"cliente", "el", false, "ya existe un cliente con ese nombre en esa organización", "el cliente no existe"}
	kindProject = kind{"proyecto", "el", false, "ya existe un proyecto con ese nombre; usa «nexus project merge» para unirlos", "el proyecto no existe"}
)

// done conjuga un participio según el género del elemento: done("cread") da "creada" o "creado".
func (k kind) done(stem string) string {
	if k.feminine {
		return stem + "a"
	}
	return stem + "o"
}

// localizeCatalog añade al error el tipo de elemento afectado para que el mensaje sea preciso.
func localizeCatalog(k kind, err error) error {
	switch {
	case errors.Is(err, catalog.ErrNotFound):
		return errors.New(k.missing)
	case errors.Is(err, catalog.ErrDuplicateName):
		return errors.New(k.duplicate)
	default:
		return localize(err)
	}
}
