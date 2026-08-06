package teamsauth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveChromeExecutable(t *testing.T) {
	t.Run("direct executable", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "chromium")
		if err := os.WriteFile(path, nil, 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := resolveChromeExecutable(path)
		if err != nil || got != path {
			t.Fatalf("resolveChromeExecutable() = %q, %v", got, err)
		}
	})

	t.Run("app bundle named executable", func(t *testing.T) {
		bundle := filepath.Join(t.TempDir(), "Chromium.app")
		executable := filepath.Join(bundle, "Contents", "MacOS", "Chromium")
		if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(executable, nil, 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := resolveChromeExecutable(bundle)
		if err != nil || got != executable {
			t.Fatalf("resolveChromeExecutable() = %q, %v", got, err)
		}
	})

	t.Run("app bundle sole executable", func(t *testing.T) {
		bundle := filepath.Join(t.TempDir(), "Helium.app")
		executable := filepath.Join(bundle, "Contents", "MacOS", "helium-browser")
		if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(executable, nil, 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := resolveChromeExecutable(bundle)
		if err != nil || got != executable {
			t.Fatalf("resolveChromeExecutable() = %q, %v", got, err)
		}
	})
}
