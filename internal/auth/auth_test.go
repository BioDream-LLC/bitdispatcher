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

// The public demonstration account signs in without its password, never as an administrator or a field crew,
// and wrong passwords typed for it do not lock the button out.
func TestDemoAccount(t *testing.T) {
	s, _ := Open(t.TempDir())
	s.Save("admin", "Admin", Admin, GeneratePassword(), false)
	s.Save("demo", "Demo dispatcher", Dispatcher, GeneratePassword(), false)
	s.Save("other", "Other", Viewer, GeneratePassword(), false)
	if _, ok := s.Demo(); ok {
		t.Fatal("a demonstration account before one was named")
	}
	if _, err := s.LoginDemo("", ""); err == nil {
		t.Fatal("signed in as a demonstration account that does not exist")
	}
	if err := s.SetDemo("admin", true); err == nil {
		t.Fatal("an administrator became the demonstration account")
	}
	if err := s.SetDemo("other", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDemo("demo", true); err != nil {
		t.Fatal(err)
	}
	if d, ok := s.Demo(); !ok || d.Username != "demo" {
		t.Fatalf("demo %+v %v", d, ok)
	}
	for _, u := range s.List() {
		if u.Username == "other" && u.Demo {
			t.Fatal("two demonstration accounts")
		}
	}
	for i := 0; i < 6; i++ {
		s.Login("demo", "wrong-password-xx", "", "")
	}
	se, err := s.LoginDemo("WEB-1", "203.0.113.9")
	if err != nil || se.Username != "demo" || se.Role != Dispatcher || se.Console != "WEB-1" {
		t.Fatalf("%+v %v", se, err)
	}
	if err := s.Save("demo", "Demo", Admin, "", false); err == nil {
		t.Fatal("the demonstration account was made an administrator")
	}
	s.Save("demo", "Demo", Dispatcher, "", true)
	if _, err := s.LoginDemo("", ""); err == nil {
		t.Fatal("a disabled demonstration account signed in")
	}
}
