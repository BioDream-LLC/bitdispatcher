//go:build !windows

package main

// runService is Windows only; elsewhere bitdispatcher always runs in the foreground.
func runService([]string) (int, bool) { return 0, false }
