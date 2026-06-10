package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDatabasePathUsesExplicitPath(t *testing.T) {
	t.Setenv(databaseEnvVar, filepath.Join(t.TempDir(), "env", databaseFile))
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg"))

	want := filepath.Join(t.TempDir(), "explicit", "custom.db")
	got, err := ResolveDatabasePath(want)
	if err != nil {
		t.Fatalf("resolve database path: %v", err)
	}
	if got != want {
		t.Fatalf("db path = %q, want %q", got, want)
	}
	assertDirExists(t, filepath.Dir(want))
}

func TestResolveDatabasePathUsesEnvWhenExplicitAbsent(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg"))

	want := filepath.Join(t.TempDir(), "env", databaseFile)
	t.Setenv(databaseEnvVar, want)

	got, err := ResolveDatabasePath("")
	if err != nil {
		t.Fatalf("resolve database path: %v", err)
	}
	if got != want {
		t.Fatalf("db path = %q, want %q", got, want)
	}
	assertDirExists(t, filepath.Dir(want))
}

func TestResolveDatabasePathUsesXDGDataHome(t *testing.T) {
	t.Setenv(databaseEnvVar, "")

	xdgDataHome := filepath.Join(t.TempDir(), "xdg-data")
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	want := filepath.Join(xdgDataHome, databaseDir, databaseFile)
	got, err := ResolveDatabasePath("")
	if err != nil {
		t.Fatalf("resolve database path: %v", err)
	}
	if got != want {
		t.Fatalf("db path = %q, want %q", got, want)
	}
	assertDirExists(t, filepath.Dir(want))
}

func TestResolveDatabasePathFallsBackToLocalShare(t *testing.T) {
	t.Setenv(databaseEnvVar, "")
	t.Setenv("XDG_DATA_HOME", "")

	home := t.TempDir()
	t.Setenv("HOME", home)

	want := filepath.Join(home, ".local", "share", databaseDir, databaseFile)
	got, err := ResolveDatabasePath("")
	if err != nil {
		t.Fatalf("resolve database path: %v", err)
	}
	if got != want {
		t.Fatalf("db path = %q, want %q", got, want)
	}
	assertDirExists(t, filepath.Dir(want))
}

func assertDirExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat parent dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("parent path %q is not a directory", path)
	}
}
