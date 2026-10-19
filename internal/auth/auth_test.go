package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoginLockoutAndPersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil || !s.Empty() {
		t.Fatal(err)
	}
	if err := s.Save("admin", "Admin", Admin, "short", false); err == nil {
		t.Fatal("a short password was accepted")
	}
	pw := GeneratePassword()
	if err := s.Save("admin", "Admin", Admin, pw, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("admin", "Admin", Viewer, "", false); err == nil {
		t.Fatal("the last administrator was demoted")
	}
	se, err := s.Login("ADMIN", pw, "D1", "127.0.0.1")
	if err != nil || se.Role != Admin || se.Console != "D1" {
		t.Fatalf("%+v %v", se, err)
	}
	if got, ok := s.Session(se.ID); !ok || got.Username != "admin" {
		t.Fatal("session not found")
	}
	for i := 0; i < 5; i++ {
		if _, err := s.Login("admin", "wrong-password-xx", "", ""); !errors.Is(err, ErrBadLogin) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, err := s.Login("admin", pw, "", ""); !errors.Is(err, ErrLocked) {
		t.Fatalf("not locked: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "users.json"))
	if strings.Contains(string(b), pw) {
		t.Fatal("the password is in the file")
	}
	fi, _ := os.Stat(filepath.Join(dir, "users.json"))
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("users.json mode %v", fi.Mode())
	}
	s2, _ := Open(dir)
	if s2.Empty() || s2.List()[0].Role != Admin {
		t.Fatal("not persisted")
	}
}

func TestSessionExpiry(t *testing.T) {
	s, _ := Open(t.TempDir())
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	pw := GeneratePassword()
	s.Save("d1", "Dispatcher", Dispatcher, pw, false)
	se, _ := s.Login("d1", pw, "", "")
	now = now.Add(13 * time.Hour)
	if _, ok := s.Session(se.ID); ok {
		t.Fatal("an idle session survived")
	}
}

func TestRoles(t *testing.T) {
	if !AtLeast(Admin, Dispatcher) || AtLeast(Viewer, Dispatcher) || AtLeast("", Viewer) {
		t.Fatal("role order")
	}
}
