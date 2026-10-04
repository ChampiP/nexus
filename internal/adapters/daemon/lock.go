package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ErrAlreadyRunning indica que otra instancia del daemon tiene el candado.
var ErrAlreadyRunning = errors.New("nexus daemon ya está en ejecución")

// LockPath es la ruta del candado: el directorio de runtime del usuario, o el temporal como respaldo.
func LockPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "nexus-daemon.lock")
}

// Lock toma un flock exclusivo no bloqueante sobre path y devuelve la función que lo libera.
func Lock(path string) (release func(), err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("abrir candado: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("tomar candado: %w", err)
	}
	return func() { f.Close() }, nil
}
