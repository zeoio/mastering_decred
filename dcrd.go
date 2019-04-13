package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"

	"github.com/decred/dcrd/blockchain/indexers"
	"github.com/decred/dcrd/internal/limits"
	"github.com/decred/dcrd/internal/version"
)

var cfg *config

// serviceStartOfDayChan is only used by Windows when the code is running as a
// service.  It signals the service code that startup has completed.  Notice
// that it uses a buffered channel so the caller will not be blocked when the
// service is not running.
var serviceStartOfDayChan = make(chan *config, 1)

// dcrdMain is the real main function for dcrd.  It is necessary to work around
// the fact that deferred functions do not run when os.Exit() is called.
func dcrdMain() error {
	// Load configuration and parse command line.  This function also
	// initializes logging and configures it accordingly.
	tcfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	cfg = tcfg
	defer func() {
		if logRotator != nil {
			logRotator.Close()
		}
	}()

	// Get a context that will be canceled when a shutdown signal has been
	// triggered either from an OS signal such as SIGINT (Ctrl+C) or from
	// another subsystem such as the RPC server.
	ctx := shutdownListener()
	defer dcrdLog.Info("Shutdown complete")

	if cfg.NoFileLogging {
		dcrdLog.Info("File logging disabled")
	}

	if cfg.LifetimeEvents {
		lifetimeNotifier = newLifetimeEventServer(outgoingPipeMessages)
	}

	if cfg.PipeRx != 0 {
		go serviceControlPipeRx(uintptr(cfg.PipeRx))
	}
	if cfg.PipeTx != 0 {
		go serviceControlPipeTx(uintptr(cfg.PipeTx))
	} else {
		go drainOutgoingPipeMessages()
	}

	// Return now if a shutdown signal was triggered.
	if shutdownRequested(ctx) {
		return nil
	}

	// Load the block database.
	db, err := loadBlockDB()
	if err != nil {
		dcrdLog.Errorf("%v", err)
		return err
	}
	defer func() {
		// Ensure the database is sync'd and closed on shutdown.
		dcrdLog.Infof("Gracefully shutting down the database...")
		db.Close()
	}()

	// Return now if a shutdown signal was triggered.
	if shutdownRequested(ctx) {
		return nil
	}

	// Drop indexes and exit if requested.
	//
	// NOTE: The order is important here because dropping the tx index also
	// drops the address index since it relies on it.
	if cfg.DropAddrIndex {
		if err := indexers.DropAddrIndex(db, ctx.Done()); err != nil {
			dcrdLog.Errorf("%v", err)
			return err
		}

		return nil
	}
	if cfg.DropTxIndex {
		if err := indexers.DropTxIndex(db, ctx.Done()); err != nil {
			dcrdLog.Errorf("%v", err)
			return err
		}

		return nil
	}
	if cfg.DropExistsAddrIndex {
		if err := indexers.DropExistsAddrIndex(db, ctx.Done()); err != nil {
			dcrdLog.Errorf("%v", err)
			return err
		}

		return nil
	}
	if cfg.DropCFIndex {
		if err := indexers.DropCfIndex(db, ctx.Done()); err != nil {
			dcrdLog.Errorf("%v", err)
			return err
		}

		return nil
	}

	// Create server and start it.
	lifetimeNotifier.notifyStartupEvent(lifetimeEventP2PServer)
	server, err := newServer(cfg.Listeners, db, activeNetParams.Params,
		cfg.DataDir, ctx.Done())
	if err != nil {
		// TODO(oga) this logging could do with some beautifying.
		dcrdLog.Errorf("Unable to start server on %v: %v",
			cfg.Listeners, err)
		return err
	}
	defer func() {
		lifetimeNotifier.notifyShutdownEvent(lifetimeEventP2PServer)
		dcrdLog.Infof("Gracefully shutting down the server...")
		server.Stop()
		server.WaitForShutdown()
		srvrLog.Infof("Server shutdown complete")
	}()

	server.Start()

	if shutdownRequested(ctx) {
		return nil
	}

	lifetimeNotifier.notifyStartupComplete()

	// Signal the Windows service (if running) that startup has completed.
	serviceStartOfDayChan <- cfg

	// Wait until the interrupt signal is received from an OS signal or
	// shutdown is requested through one of the subsystems such as the RPC
	// server.
	<-ctx.Done()
	return nil
}

func main() {
	// Use all processor cores.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Block and transaction processing can cause bursty allocations.  This
	// limits the garbage collector from excessively overallocating during
	// bursts.  This value was arrived at with the help of profiling live
	// usage.
	debug.SetGCPercent(20)

	// Up some limits.
	if err := limits.SetLimits(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set limits: %v\n", err)
		os.Exit(1)
	}

	// Work around defer not working after os.Exit()
	if err := dcrdMain(); err != nil {
		os.Exit(1)
	}
}
