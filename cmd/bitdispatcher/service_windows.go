//go:build windows

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows/svc"
)

// serviceName is the Windows service the installer registers (packaging/windows).
const serviceName = "bitdispatcher"

// runService runs bitdispatcher under the Windows Service Control Manager when the SCM started it.
// It reports false when started from a console, so main runs as usual.
func runService(args []string) (int, bool) {
	is, err := svc.IsWindowsService()
	if err != nil || !is {
		return 0, false
	}
	logw, closeLog := serviceLog(serviceDataDir(args))
	defer closeLog()
	if err := svc.Run(serviceName, &service{args: args, logw: logw}); err != nil {
		fmt.Fprintf(logw, "service: %v\n", err)
		return 1, true
	}
	return 0, true
}

type service struct {
	args []string
	logw io.Writer
}

// Execute serves until the SCM asks the service to stop, or serving ends by itself. A failure is
// reported as a service-specific exit code, so the SCM's recovery actions restart it.
func (s *service) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rootCtx = ctx
	done := make(chan int, 1)
	go func() { done <- run(s.args, s.logw, s.logw) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending, WaitHint: 15000}
				cancel()
				select {
				case <-done:
				case <-time.After(15 * time.Second):
				}
				return false, 0
			}
		case code := <-done:
			if code != 0 {
				return true, uint32(code)
			}
			return false, 0
		}
	}
}

// serviceDataDir finds the -data flag the service was registered with.
func serviceDataDir(args []string) string {
	for i, a := range args {
		a = strings.TrimLeft(a, "-")
		if a == "data" && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, "data="); ok {
			return v
		}
	}
	return defaultDataDir()
}

// serviceLog opens <data>\logs\bitdispatcher.log for a service, which has no console. The previous
// log is kept as bitdispatcher.log.1 once it passes 20 MB. The installer limits the data folder to
// SYSTEM and Administrators, because the log can hold a first-start password.
func serviceLog(dataDir string) (io.Writer, func()) {
	dir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return io.Discard, func() {}
	}
	p := filepath.Join(dir, "bitdispatcher.log")
	if fi, err := os.Stat(p); err == nil && fi.Size() > 20<<20 {
		os.Rename(p, p+".1")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return io.Discard, func() {}
	}
	return f, func() { f.Close() }
}
