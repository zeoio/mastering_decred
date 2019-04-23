package main

import (
	"context"
	"os"
	"os/signal"
)

// shutdownRequestChannel is used to initiate shutdown from one of the
// subsystems using the same code paths as when an interrupt signal is received.
var shutdownRequestChannel = make(chan struct{})

// interruptSignals defines the default signals to catch in order to do a proper
// shutdown.  This may be modified during init depending on the platform.
// 中断信号
var interruptSignals = []os.Signal{os.Interrupt}

// shutdownListener listens for OS Signals such as SIGINT (Ctrl+C) and shutdown
// requests from shutdownRequestChannel.  It returns a context that is canceled
// when either signal is received.
// 当监听到系统中断信号和关闭请求通道时，则取消监听上下文
func shutdownListener() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		interruptChannel := make(chan os.Signal, 1)
		signal.Notify(interruptChannel, interruptSignals...)

		// Listen for initial shutdown signal and cancel the returned context.
		select {
		case sig := <-interruptChannel:
			dcrdLog.Infof("Received signal (%s).  Shutting down...", sig)

		case <-shutdownRequestChannel:
			dcrdLog.Infof("Shutdown requested.  Shutting down...")
		}
		cancel()

		// Listen for repeated signals and display a message so the user
		// knows the shutdown is in progress and the process is not
		// hung.
		for {
			select {
			case sig := <-interruptChannel:
				dcrdLog.Infof("Received signal (%s).  Already "+
					"shutting down...", sig)

			case <-shutdownRequestChannel:
				dcrdLog.Info("Shutdown requested.  Already " +
					"shutting down...")
			}
		}
	}()

	return ctx
}

// shutdownRequested returns true when the context returned by shutdownListener
// was canceled.  This simplifies early shutdown slightly since the caller can
// just use an if statement instead of a select.
// 当监听关闭的上下文取消的时候返回true, 否则返回false
func shutdownRequested(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
	}

	return false
}
