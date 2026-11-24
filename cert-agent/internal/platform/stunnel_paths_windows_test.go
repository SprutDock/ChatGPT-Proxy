package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureStunnelJunction(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(root, "ascii alias")
	first := filepath.Join(root, "Проверка & %PATH% !PATH!")
	second := filepath.Join(root, "Другой профиль")
	for _, target := range []string{first, second} {
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(first, "marker"), []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureStunnelJunction(alias, first); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(filepath.Join(alias, "marker")); err != nil || string(content) != "first" {
		t.Fatalf("junction did not reach the Cyrillic path: %q, %v", content, err)
	}
	original, err := os.Lstat(alias)
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureStunnelJunction(alias, first); err != nil {
		t.Fatal(err)
	}
	reused, err := os.Lstat(alias)
	if err != nil || !os.SameFile(original, reused) {
		t.Fatalf("existing junction was not reused: %v", err)
	}
	if err := ensureStunnelJunction(alias, second); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(first, "marker")); err != nil {
		t.Fatalf("replacing the junction changed the old target: %v", err)
	}
	if err := os.Remove(second); err != nil {
		t.Fatal(err)
	}
	if err := ensureStunnelJunction(alias, first); err != nil {
		t.Fatalf("could not replace a dangling junction: %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(alias, "marker")); err != nil || string(content) != "first" {
		t.Fatalf("replaced junction did not reach its target: %q, %v", content, err)
	}
	if err := secureStunnelDirectory(alias, ""); err == nil {
		t.Fatal("a junction was accepted as the protected directory")
	}
	if err := ensureStunnelJunction(first, root); err == nil {
		t.Fatal("an ordinary directory was accepted as a junction")
	}
	if _, err := os.Stat(filepath.Join(first, "marker")); err != nil {
		t.Fatalf("the ordinary directory was modified: %v", err)
	}
}
