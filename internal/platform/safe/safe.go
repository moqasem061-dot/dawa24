package safe

import (
	"log/slog"
	"runtime/debug"
)

// Go runs fn in a new goroutine and isolates any panic, ensuring that a panic
// cannot terminate the entire runtime process. If fn panics, the panic and stack
// trace are logged at Error level.
func Go(log *slog.Logger, name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())
				if log != nil {
					log.Error("safe.Go: background routine panicked",
						"routine", name,
						"panic", r,
						"stack", stack,
					)
				} else {
					slog.Error("safe.Go: background routine panicked",
						"routine", name,
						"panic", r,
						"stack", stack,
					)
				}
			}
		}()
		fn()
	}()
}

// GoWithRecover runs fn in a new goroutine with a custom onPanic handler.
// If onPanic panics, that panic is also safely caught and logged.
func GoWithRecover(log *slog.Logger, name string, fn func(), onPanic func(p any)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())
				if log != nil {
					log.Error("safe.GoWithRecover: background routine panicked",
						"routine", name,
						"panic", r,
						"stack", stack,
					)
				} else {
					slog.Error("safe.GoWithRecover: background routine panicked",
						"routine", name,
						"panic", r,
						"stack", stack,
					)
				}
				if onPanic != nil {
					defer func() {
						if p2 := recover(); p2 != nil {
							if log != nil {
								log.Error("safe.GoWithRecover: onPanic handler panicked",
									"routine", name,
									"panic", p2,
								)
							}
						}
					}()
					onPanic(r)
				}
			}
		}()
		fn()
	}()
}
