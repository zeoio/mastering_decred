package addrmgr

import (
	"github.com/decred/slog"
)

// log is a logger that is initialized with no output filters.  This
// means the package will not perform any logging by default until the caller
// requests it.
// The default amount of logging is none.
var log = slog.Disabled

// DisableLog disables all library log output.  Logging output is disabled
// by default until UseLogger is called.
//
// Deprecated: Use UseLogger(slog.Disabled) instead.
func DisableLog() {
	log = slog.Disabled
}

// UseLogger uses a specified Logger to output package logging info.
// This should be used in preference to SetLogWriter if the caller is also
// using slog.
func UseLogger(logger slog.Logger) {
	log = logger
}
