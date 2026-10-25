//go:build !unix

package api

func freeBytes(string) (uint64, bool) { return 0, false }
