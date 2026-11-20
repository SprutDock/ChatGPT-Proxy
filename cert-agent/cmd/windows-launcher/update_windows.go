package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	githubLatestRelease = "https://api.github.com/repos/SprutDock/ChatGPT-Proxy/releases/latest"
	maxUpdateArchive    = 100 << 20
)

type githubRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	Digest      string `json:"digest"`
	DownloadURL string `json:"browser_download_url"`
}

type nativeSystemInfo struct {
	ProcessorArchitecture uint16
	Reserved              uint16
	PageSize              uint32
	MinimumAddress        uintptr
	MaximumAddress        uintptr
	ActiveProcessorMask   uintptr
	ProcessorCount        uint32
	ProcessorType         uint32
	AllocationGranularity uint32
	ProcessorLevel        uint16
	ProcessorRevision     uint16
}

var getNativeSystemInfo = syscall.NewLazyDLL("kernel32.dll").NewProc("GetNativeSystemInfo")

func checkForUpdate(root string) (bool, error) {
	currentVersionData, err := os.ReadFile(supportFile(root, "version.txt"))
	if err != nil {
		return false, fmt.Errorf("не удалось прочитать версию приложения")
	}
	currentVersion := strings.TrimSpace(string(currentVersionData))
	release, err := latestRelease()
	if err != nil {
		launcherLog("update check skipped: GitHub is unavailable")
		return false, nil
	}
	version := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	if compareVersions(version, currentVersion) <= 0 {
		return false, nil
	}
	architecture, err := nativeArchitecture()
	if err != nil {
		launcherLog("update check skipped: unsupported Windows architecture")
		return false, nil
	}
	assetName := "ChatGPT-Proxy-Windows-" + architecture + ".zip"
	var asset *releaseAsset
	for index := range release.Assets {
		if release.Assets[index].Name == assetName {
			asset = &release.Assets[index]
			break
		}
	}
	if asset == nil {
		launcherLog("update check skipped: matching Windows asset is unavailable")
		return false, nil
	}
	if showMessage("Доступно обновление ChatGPT Proxy", "Установлена версия "+currentVersion+". Доступна версия "+version+". Установить обновление?", 0x4|0x40) != 6 {
		return false, nil
	}
	stage, err := downloadAndExtractUpdate(*asset, version, architecture)
	if err != nil {
		launcherLog("update package rejected or unavailable")
		showMessage("Не удалось подготовить обновление", "Архив обновления не прошёл проверку. Запускается установленная версия.", 0x10)
		return false, nil
	}
	if err := startUpdateHelper(stage, root); err != nil {
		_ = os.RemoveAll(filepath.Dir(stage))
		return false, fmt.Errorf("не удалось запустить установщик обновления")
	}
	launcherLog("update queued version=" + version)
	return true, nil
}

func latestRelease() (githubRelease, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, githubLatestRelease, nil)
	if err != nil {
		return githubRelease{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "ChatGPT-Proxy")
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(request *http.Request, _ []*http.Request) error {
			if request.URL.Scheme != "https" || !strings.EqualFold(request.URL.Hostname(), "api.github.com") {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return githubRelease{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return githubRelease{}, fmt.Errorf("release service returned HTTP %d", response.StatusCode)
	}
	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release); err != nil {
		return githubRelease{}, err
	}
	return release, nil
}

func compareVersions(left, right string) int {
	leftParts, ok := versionParts(left)
	if !ok {
		return 0
	}
	rightParts, ok := versionParts(right)
	if !ok {
		return 0
	}
	for index := 0; index < 4; index++ {
		if leftParts[index] > rightParts[index] {
			return 1
		}
		if leftParts[index] < rightParts[index] {
			return -1
		}
	}
	return 0
}

func versionParts(value string) ([4]int, bool) {
	var parts [4]int
	segments := strings.Split(value, ".")
	if len(segments) < 2 || len(segments) > 4 {
		return parts, false
	}
	for index, segment := range segments {
		if segment == "" {
			return parts, false
		}
		number, err := strconv.Atoi(segment)
		if err != nil || number < 0 || number > 1_000_000 {
			return parts, false
		}
		parts[index] = number
	}
	return parts, true
}

func nativeArchitecture() (string, error) {
	var information nativeSystemInfo
	getNativeSystemInfo.Call(uintptr(unsafe.Pointer(&information)))
	switch information.ProcessorArchitecture {
	case 9:
		return "x64", nil
	case 12:
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported Windows architecture")
	}
}

func startUpdateHelper(stage, root string) error {
	helper := filepath.Join(stage, "ChatGPT Proxy.exe")
	command := exec.Command(helper, "--apply-update", stage, root, strconv.Itoa(os.Getpid()))
	command.Dir = stage
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := command.Start(); err != nil {
		return err
	}
	_ = command.Process.Release()
	return nil
}

func cleanupPreviousUpdateStage() {
	stateDirectory := userStateDirectory()
	marker := filepath.Join(stateDirectory, "update-cleanup.json")
	data, err := os.ReadFile(marker)
	if err != nil {
		return
	}
	var cleanup struct {
		Path string `json:"path"`
		PID  uint32 `json:"pid"`
	}
	if json.Unmarshal(data, &cleanup) != nil || !validUpdateTempPath(cleanup.Path) {
		return
	}
	if cleanup.PID != 0 {
		if waitForProcessExit(cleanup.PID, 10*time.Second) != nil {
			return
		}
	}
	_ = os.RemoveAll(cleanup.Path)
	_ = os.Remove(marker)
}

func userStateDirectory() string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		home, _ := os.UserHomeDir()
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(localAppData, "SprutDock", "ChatGPT Proxy")
}

func validUpdateTempPath(path string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	clean := filepath.Clean(path)
	return filepath.Dir(clean) == filepath.Clean(os.TempDir()) && strings.HasPrefix(strings.ToLower(filepath.Base(clean)), "sprutdock-proxy-update-")
}
