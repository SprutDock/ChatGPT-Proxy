package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxExpandedUpdate = 250 << 20

func downloadAndExtractUpdate(asset releaseAsset, version, architecture string) (string, error) {
	expectedName := "ChatGPT-Proxy-Windows-" + architecture + ".zip"
	if asset.Name != expectedName || asset.Size <= 0 || asset.Size > maxUpdateArchive || len(asset.Digest) != 71 || !strings.HasPrefix(strings.ToLower(asset.Digest), "sha256:") {
		return "", fmt.Errorf("release asset metadata is invalid")
	}
	digest, err := hex.DecodeString(asset.Digest[7:])
	if err != nil || len(digest) != sha256.Size {
		return "", fmt.Errorf("release asset digest is invalid")
	}
	if !validReleaseURL(asset.DownloadURL) {
		return "", fmt.Errorf("release asset URL is invalid")
	}
	root, err := os.MkdirTemp("", "sprutdock-proxy-update-")
	if err != nil {
		return "", err
	}
	archivePath := filepath.Join(root, asset.Name)
	if err := downloadReleaseAsset(asset, archivePath, digest); err != nil {
		_ = os.RemoveAll(root)
		return "", err
	}
	packageDirectory := filepath.Join(root, "package")
	if err := os.Mkdir(packageDirectory, 0o700); err != nil {
		_ = os.RemoveAll(root)
		return "", err
	}
	if err := extractUpdateArchive(archivePath, packageDirectory); err != nil {
		_ = os.RemoveAll(root)
		return "", err
	}
	if err := checkPackageFiles(packageDirectory); err != nil {
		_ = os.RemoveAll(root)
		return "", err
	}
	versionData, err := os.ReadFile(supportFile(packageDirectory, "version.txt"))
	if err != nil || strings.TrimSpace(string(versionData)) != version {
		_ = os.RemoveAll(root)
		return "", fmt.Errorf("package version does not match the release")
	}
	return packageDirectory, nil
}

func validReleaseURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.User == nil && parsed.Port() == "" && strings.EqualFold(parsed.Hostname(), "github.com") && strings.HasPrefix(parsed.Path, "/SprutDock/ChatGPT-Proxy/releases/download/")
}

func downloadReleaseAsset(asset releaseAsset, destination string, expectedDigest []byte) error {
	request, err := http.NewRequest(http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "ChatGPT-Proxy")
	client := &http.Client{
		Timeout: 90 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 || request.URL.Scheme != "https" || request.URL.User != nil || request.URL.Port() != "" || !allowedGitHubDownloadHost(request.URL.Hostname()) {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("release download returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > 0 && response.ContentLength != asset.Size {
		return fmt.Errorf("release download size does not match")
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, asset.Size+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != asset.Size || !equalBytes(hash.Sum(nil), expectedDigest) {
		return fmt.Errorf("release download failed its size or digest check")
	}
	return nil
}

func allowedGitHubDownloadHost(host string) bool {
	host = strings.ToLower(host)
	return host == "github.com" || host == "release-assets.githubusercontent.com" || host == "github-releases.githubusercontent.com" || host == "objects.githubusercontent.com"
}

func extractUpdateArchive(archivePath, destination string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("release archive is invalid")
	}
	defer archive.Close()
	if len(archive.File) == 0 || len(archive.File) > 512 {
		return fmt.Errorf("release archive contains an invalid number of files")
	}
	seen := make(map[string]struct{}, len(archive.File))
	var expanded uint64
	for _, entry := range archive.File {
		relative, directory, err := safeArchivePath(entry.Name)
		if err != nil {
			return err
		}
		directory = directory || entry.Mode().IsDir()
		key := strings.ToLower(relative)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("release archive contains duplicate paths")
		}
		seen[key] = struct{}{}
		if entry.Mode()&os.ModeSymlink != 0 || entry.Mode()&os.ModeType != 0 && !directory && !entry.Mode().IsRegular() {
			return fmt.Errorf("release archive contains an unsupported file type")
		}
		target := filepath.Join(destination, relative)
		if directory {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}
		if entry.UncompressedSize64 > maxUpdateArchive || expanded+entry.UncompressedSize64 > maxExpandedUpdate {
			return fmt.Errorf("release archive expands beyond the allowed size")
		}
		expanded += entry.UncompressedSize64
		if err := extractUpdateFile(entry, target); err != nil {
			return err
		}
	}
	return nil
}

func safeArchivePath(value string) (string, bool, error) {
	if value == "" || strings.ContainsRune(value, 0) {
		return "", false, fmt.Errorf("release archive contains an invalid path")
	}
	value = strings.ReplaceAll(value, `\`, "/")
	directory := strings.HasSuffix(value, "/")
	value = strings.TrimSuffix(value, "/")
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, ":") || filepath.IsAbs(value) {
		return "", false, fmt.Errorf("release archive contains an absolute path")
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return "", false, fmt.Errorf("release archive contains an invalid path component")
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if windowsReservedName(base) {
			return "", false, fmt.Errorf("release archive contains a reserved Windows path")
		}
	}
	return filepath.Join(parts...), directory, nil
}

func windowsReservedName(value string) bool {
	if value == "CON" || value == "PRN" || value == "AUX" || value == "NUL" {
		return true
	}
	if len(value) == 4 && (strings.HasPrefix(value, "COM") || strings.HasPrefix(value, "LPT")) && value[3] >= '1' && value[3] <= '9' {
		return true
	}
	return false
}

func extractUpdateFile(entry *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	source, err := entry.Open()
	if err != nil {
		return fmt.Errorf("release archive file cannot be opened")
	}
	defer source.Close()
	destination, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(destination, io.LimitReader(source, int64(entry.UncompressedSize64)+1))
	closeErr := destination.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if uint64(written) != entry.UncompressedSize64 {
		return fmt.Errorf("release archive file size does not match")
	}
	return nil
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var difference byte
	for index := range left {
		difference |= left[index] ^ right[index]
	}
	return difference == 0
}
