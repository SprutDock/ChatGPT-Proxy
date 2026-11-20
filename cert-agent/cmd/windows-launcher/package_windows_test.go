package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckPackageFilesUsesSupportFolder(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ChatGPT Proxy.exe"), []byte("exe"), 0o600); err != nil {
		t.Fatal(err)
	}
	resources := filepath.Join(root, packageResourcesDirectory)
	for _, name := range []string{
		"cert-agent.exe", "stunnel.exe", "windows-launcher.ps1", "windows-update.ps1",
		"client-ca-chain.pem", "server-ca.pem", "version.txt", filepath.Join("config", "openssl.cnf"),
		filepath.Join("config", "fipsmodule.cnf"), filepath.Join("ossl-modules", "fips.dll"),
	} {
		path := filepath.Join(resources, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("file"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkPackageFiles(root); err != nil {
		t.Fatal(err)
	}
	if got := supportFile(root, "version.txt"); got != filepath.Join(resources, "version.txt") {
		t.Fatalf("supportFile = %q, want nested resource path", got)
	}
}

func TestSupportFileKeepsLegacyFlatPackage(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "version.txt")
	if err := os.WriteFile(path, []byte("1.0.6"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := supportFile(root, "version.txt"); got != path {
		t.Fatalf("supportFile = %q, want legacy path", got)
	}
}
