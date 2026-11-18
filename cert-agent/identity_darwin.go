package main

import (
	"errors"
	"os"
	"path/filepath"
)

func setActive(p Paths, serial string) (string, error) {
	releasePath := filepath.Join(p.Releases, serial)
	if _, err := os.Stat(filepath.Join(releasePath, "client.pem")); err != nil {
		return "", err
	}
	if err := ensurePrivateDir(filepath.Dir(p.ActiveLink)); err != nil {
		return "", err
	}
	previous, _ := os.Readlink(p.ActiveLink)
	relativePath, err := filepath.Rel(filepath.Dir(p.ActiveLink), releasePath)
	if err != nil {
		return previous, err
	}
	temporary := p.ActiveLink + ".new"
	_ = os.Remove(temporary)
	if err := os.Symlink(relativePath, temporary); err != nil {
		return previous, err
	}
	if err := os.Rename(temporary, p.ActiveLink); err != nil {
		_ = os.Remove(temporary)
		return previous, err
	}
	return previous, nil
}

func restoreActive(p Paths, previous string) error {
	if previous == "" {
		if err := os.Remove(p.ActiveLink); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	temporary := p.ActiveLink + ".restore"
	_ = os.Remove(temporary)
	if err := os.Symlink(previous, temporary); err != nil {
		return err
	}
	return os.Rename(temporary, p.ActiveLink)
}
