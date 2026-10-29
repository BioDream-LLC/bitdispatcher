//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// notifyQueueToggle delivers SIGUSR1, which switches the PSAP queue between Active and Full.
func notifyQueueToggle(c chan os.Signal) { signal.Notify(c, syscall.SIGUSR1) }
