package mapdata

import (
	"os"
	"path/filepath"
	"testing"
)

// The extract's MD5 is checked even in FIPS 140-only mode (GODEBUG=fips140=only), where MD5 is refused
// for anything that protects data.
func TestMD5File(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x")
	os.WriteFile(p, []byte("bitdispatcher"), 0o600)
	got, err := md5File(p)
	if err != nil || got != "c9bdd94061f78cb8ceaf8d9ac4722b4c" {
		t.Fatalf("md5File = %q, %v", got, err)
	}
}
