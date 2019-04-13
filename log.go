package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/decred/dcrd/addrmgr"
	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/indexers"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/connmgr"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/fees"
	"github.com/decred/dcrd/mempool/v2"
	"github.com/decred/dcrd/peer"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/slog"
	"github.com/jrick/logrotate/rotator"
)

// logWriter implements an io.Writer that outputs to both standard output and
// the write-end pipe of an initialized log rotator.
type logWriter struct{}

func (logWriter) Write(p []byte) (n int, err error) {
	os.Stdout.Write(p)
	if logRotator != nil {
		logRotator.Write(p)
	}
	return len(p), nil
}

// Loggers per subsystem.  A single backend logger is created and all subsytem
// loggers created from it will write to the backend.  When adding new
// subsystems, add the subsystem logger variable here and to the
// subsystemLoggers map.
//
// Loggers can not be used before the log rotator has been initialized with a
// log file.  This must be performed early during application startup by calling
// initLogRotator.
var (
	// backendLog is the logging backend used to create all subsystem loggers.
	// The backend must not be used before the log rotator has been initialized,
	// or data races and/or nil pointer dereferences will occur.
	backendLog = slog.NewBackend(logWriter{})

	adxrLog = backendLog.Logger("ADXR")
	amgrLog = backendLog.Logger("AMGR")
	bcdbLog = backendLog.Logger("BCDB")
	bmgrLog = backendLog.Logger("BMGR")
	chanLog = backendLog.Logger("CHAN")
	cmgrLog = backendLog.Logger("CMGR")
	dcrdLog = backendLog.Logger("DCRD")
	discLog = backendLog.Logger("DISC")
	feesLog = backendLog.Logger("FEES")
	indxLog = backendLog.Logger("INDX")
	minrLog = backendLog.Logger("MINR")
	peerLog = backendLog.Logger("PEER")
	rpcsLog = backendLog.Logger("RPCS")
	scrpLog = backendLog.Logger("SCRP")
	srvrLog = backendLog.Logger("SRVR")
	stkeLog = backendLog.Logger("STKE")
	txmpLog = backendLog.Logger("TXMP")
)

// Initialize package-global logger variables.
func init() {
	addrmgr.UseLogger(amgrLog)
	blockchain.UseLogger(chanLog)
	connmgr.UseLogger(cmgrLog)
	database.UseLogger(bcdbLog)
	fees.UseLogger(feesLog)
	indexers.UseLogger(indxLog)
	mempool.UseLogger(txmpLog)
	peer.UseLogger(peerLog)
	stake.UseLogger(stkeLog)
	txscript.UseLogger(scrpLog)
}

// setLogLevel sets the logging level for provided subsystem.  Invalid
// subsystems are ignored.  Uninitialized subsystems are dynamically created as
// needed.
func setLogLevel(subsystemID string, logLevel string) {
	// Ignore invalid subsystems.
	logger, ok := subsystemLoggers[subsystemID]
	if !ok {
		return
	}

	// Defaults to info if the log level is invalid.
	level, _ := slog.LevelFromString(logLevel)
	logger.SetLevel(level)
}

// directionString is a helper function that returns a string that represents
// the direction of a connection (inbound or outbound).
func directionString(inbound bool) string {
	if inbound {
		return "inbound"
	}
	return "outbound"
}

// fatalf logs a string, then cleanly exits.
func fatalf(str string) {
	dcrdLog.Errorf("Unable to create profiler: %v", str)
	os.Stdout.Sync()
	if logRotator != nil {
		logRotator.Close()
	}
	os.Exit(1)
}
