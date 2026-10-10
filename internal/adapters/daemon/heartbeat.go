package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// FileHeartbeat conserva el último latido junto a la base de datos.
type FileHeartbeat struct{ path string }

// NewHeartbeat usa la ruta del archivo de latidos, normalmente <base>.alive.
func NewHeartbeat(path string) *FileHeartbeat { return &FileHeartbeat{path: path} }

// Last ignora un archivo ausente o dañado.
func (h *FileHeartbeat) Last() (time.Time, bool, error) {
	data, err := os.ReadFile(h.path)
	if errors.Is(err, os.ErrNotExist) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return time.Time{}, false, nil
	}
	return time.Unix(seconds, 0), true, nil
}

// Beat reemplaza el latido atómicamente para no dejar contenido parcial.
func (h *FileHeartbeat) Beat(at time.Time) error {
	file, err := os.CreateTemp(filepath.Dir(h.path), ".nexus-alive-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.WriteString(strconv.FormatInt(at.Unix(), 10)); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), h.path)
}
