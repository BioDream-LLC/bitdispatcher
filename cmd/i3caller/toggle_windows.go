//go:build windows

package main

import "os"

// notifyQueueToggle does nothing on Windows, which has no SIGUSR1; the queue stays as started.
func notifyQueueToggle(chan os.Signal) {}
