package countdown

// Timers es el puerto con el que countdown controla los timers de seguimiento.
// La raíz de composición lo implementa sobre el módulo tracking.
type Timers interface {
	StopMany(ids []int64) error
	StartBreak(label string) (entryID int64, err error)
	Stop(id int64) error
	// StartLike inicia una entrada de trabajo nueva copiando la entrada id.
	StartLike(id int64) error
	// Running indica si la entrada id sigue en curso.
	Running(id int64) (bool, error)
}

// Repository es la persistencia que necesita Service.
type Repository interface {
	Insert(Break) (Break, error)
	Active() (*Break, error)
	Finish(id, at int64) error
	SetEndsAt(id, endsAt int64) error // también limpia notified_at
	MarkNotified(id, at int64) error
}
