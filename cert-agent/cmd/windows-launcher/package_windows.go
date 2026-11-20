package main

import (
	"os"
	"path/filepath"
)

const packageResourcesDirectory = "ChatGPT Proxy Files"

func supportDirectory(root string) string {
	resources := filepath.Join(root, packageResourcesDirectory)
	if info, err := os.Stat(resources); err == nil && info.IsDir() {
		return resources
	}
	return root
}

func supportFile(root, name string) string {
	return filepath.Join(supportDirectory(root), name)
}
