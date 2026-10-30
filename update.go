package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"job_item/support"

	"github.com/urfave/cli/v2"
)

const (
	defaultUpdatePollInterval = 2 * time.Second
	defaultUpdateTimeout      = 10 * time.Minute
)

// updateBinaryName returns the OS/architecture-specific binary name,
// e.g. job_item_linux_amd64 or job_item_windows_amd64.exe.
func updateBinaryName(osName, archName string) string {
	name := fmt.Sprintf("job_item_%s_%s", osName, archName)
	if osName == "windows" {
		name += ".exe"
	}
	return name
}

// normalizeUpdateOS validates and normalizes the target OS.
func normalizeUpdateOS(osName string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(osName)) {
	case "linux":
		return "linux", nil
	case "darwin", "macos", "osx":
		return "darwin", nil
	case "windows", "win":
		return "windows", nil
	default:
		return "", fmt.Errorf("unsupported OS %q (supported: linux, darwin, windows)", osName)
	}
}

// normalizeUpdateArch maps an architecture name to the canonical Go arch used in
// both the build request and the downloaded file name.
func normalizeUpdateArch(arch string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(arch)) {
	case "amd64", "x86_64", "x64":
		return "amd64", nil
	case "386", "x86", "i386", "i686":
		return "386", nil
	case "arm64", "aarch64":
		return "arm64", nil
	case "arm":
		return "arm", nil
	default:
		return "", fmt.Errorf("unsupported architecture %q (supported: amd64, 386, arm64, arm)", arch)
	}
}

func updateEnvSeconds(name string, def int) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return time.Duration(def) * time.Second
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return time.Duration(def) * time.Second
	}
	return time.Duration(value) * time.Second
}

// runUpdate implements `job_item update`: it asks the job manager to build a fresh
// binary for the target OS/architecture, downloads it, and replaces the running binary.
func runUpdate(ctx *cli.Context) error {
	targetOS := ctx.String("os")
	if strings.TrimSpace(targetOS) == "" {
		targetOS = runtime.GOOS
	}
	targetArch := ctx.String("arch")
	if strings.TrimSpace(targetArch) == "" {
		targetArch = runtime.GOARCH
	}

	osName, err := normalizeUpdateOS(targetOS)
	if err != nil {
		return err
	}
	archName, err := normalizeUpdateArch(targetArch)
	if err != nil {
		return err
	}

	cfg, err := support.ConfigYamlSupportContructLocalOnly(support.ConfigYamlSupportConstructPropsType{
		Config_path: ctx.String("config"),
	}, support.ConfigData{})
	if err != nil {
		return fmt.Errorf("gagal membaca konfigurasi: %w", err)
	}

	endpoint := strings.TrimRight(cfg.ConfigData.End_point, "/")
	projectID := cfg.ConfigData.Credential.Project_id
	secretKey := cfg.ConfigData.Credential.Secret_key
	if projectID == "" {
		projectID = os.Getenv("JOB_ITEM_PROJECT_ID")
	}
	if secretKey == "" {
		secretKey = os.Getenv("JOB_ITEM_PROJECT_KEY")
	}
	if endpoint == "" {
		return fmt.Errorf("end_point belum diset di config; tidak bisa menghubungi job manager")
	}
	if projectID == "" || secretKey == "" {
		return fmt.Errorf("credential project belum diset (project_id/secret_key)")
	}

	outPath := ctx.String("out")
	if strings.TrimSpace(outPath) == "" {
		outPath = updateBinaryName(osName, archName)
	}
	outPath, err = filepath.Abs(outPath)
	if err != nil {
		return fmt.Errorf("path output tidak valid: %w", err)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("gagal mengetahui lokasi binary berjalan: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	if exePath == outPath {
		return fmt.Errorf("path output sama dengan binary yang sedang berjalan (%s); pilih --out lain", outPath)
	}

	if !ctx.Bool("force") {
		if _, statErr := os.Stat(outPath); statErr == nil {
			return fmt.Errorf("file output sudah ada: %s (gunakan --force untuk menimpa)", outPath)
		}
	}

	credential := projectID + "|" + secretKey

	fmt.Printf("Meminta build baru: os=%s arch=%s\n", osName, archName)
	buildID, err := requestAgentBuild(endpoint, credential, osName, archName)
	if err != nil {
		return err
	}
	fmt.Printf("Build dimulai: %s\n", buildID)

	if err := waitForAgentBuild(endpoint, credential, buildID, osName); err != nil {
		return err
	}

	fmt.Printf("Mengunduh binary -> %s\n", outPath)
	if err := downloadAgentBinary(endpoint, credential, buildID, osName, archName, outPath); err != nil {
		return err
	}

	if osName != "windows" {
		if err := os.Chmod(outPath, 0o755); err != nil {
			return fmt.Errorf("gagal set executable pada %s: %w", outPath, err)
		}
	}

	fmt.Printf("Mengganti binary berjalan (%s)...\n", exePath)
	replaceCmd := support.ReplaceApp(exePath, outPath)
	if output, err := replaceCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gagal mengganti binary: %v, output: %s", err, strings.TrimSpace(string(output)))
	}

	fmt.Printf("✓ Binary berhasil diganti: %s\n", exePath)
	fmt.Println("Restart job_item untuk menjalankan versi baru.")
	return nil
}

type updateBuildResponse struct {
	Status string `json:"status"`
	Return struct {
		BuildID string `json:"build_id"`
	} `json:"return"`
}

type updateBuildStatusResponse struct {
	Status string `json:"status"`
	Return struct {
		BuildID    string `json:"build_id"`
		Status     string `json:"status"`
		Error      string `json:"error"`
		BinaryPath string `json:"binary_path"`
	} `json:"return"`
}

func updateHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func requestAgentBuild(endpoint, credential, osName, archName string) (string, error) {
	payload, err := json.Marshal(map[string]string{"os": osName, "arch": archName})
	if err != nil {
		return "", err
	}

	url := endpoint + "/api/worker/agent/build"
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential)

	resp, err := updateHTTPClient(30 * time.Second).Do(req)
	if err != nil {
		return "", fmt.Errorf("job manager tidak dapat dihubungi (%s): %w", url, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("build ditolak (HTTP %d): %s", resp.StatusCode, extractReturnMessage(body))
	}

	var parsed updateBuildResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("respon build tidak valid: %w", err)
	}
	if parsed.Return.BuildID == "" {
		return "", fmt.Errorf("respon build tidak berisi build_id: %s", extractReturnMessage(body))
	}
	return parsed.Return.BuildID, nil
}

func waitForAgentBuild(endpoint, credential, buildID, osName string) error {
	timeout := updateEnvSeconds("JOB_ITEM_UPDATE_TIMEOUT_SECONDS", int(defaultUpdateTimeout/time.Second))
	pollInterval := updateEnvSeconds("JOB_ITEM_UPDATE_POLL_SECONDS", int(defaultUpdatePollInterval/time.Second))
	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("build %s melewati batas waktu %s", buildID, timeout)
		}

		status, buildErr, err := fetchAgentBuildStatus(endpoint, credential, buildID, osName)
		if err != nil {
			fmt.Printf("  status build belum tersedia: %v\n", err)
		} else {
			switch status {
			case "failed":
				return fmt.Errorf("build %s gagal: %s", buildID, buildErr)
			case "completed":
				fmt.Println("Build selesai.")
				return nil
			default:
				fmt.Println("  build masih berjalan...")
			}
		}
		time.Sleep(pollInterval)
	}
}

func fetchAgentBuildStatus(endpoint, credential, buildID, osName string) (string, string, error) {
	url := fmt.Sprintf("%s/api/worker/agent/build/%s?os=%s", endpoint, buildID, osName)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+credential)

	resp, err := updateHTTPClient(30 * time.Second).Do(req)
	if err != nil {
		return "", "", fmt.Errorf("job manager tidak dapat dihubungi: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	// Older servers return 404 until the binary exists; treat it as still processing.
	if resp.StatusCode == http.StatusNotFound {
		return "processing", "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("status ditolak (HTTP %d): %s", resp.StatusCode, extractReturnMessage(body))
	}

	var parsed updateBuildStatusResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", "", fmt.Errorf("respon status tidak valid: %w", err)
	}

	status := strings.ToLower(parsed.Return.Status)
	switch status {
	case "completed", "success":
		return "completed", "", nil
	case "failed", "error":
		buildErr := parsed.Return.Error
		if buildErr == "" {
			buildErr = "tanpa detail"
		}
		return "failed", buildErr, nil
	case "processing", "running", "pending", "":
		if parsed.Return.BinaryPath != "" {
			return "completed", "", nil
		}
		return "processing", "", nil
	default:
		return status, parsed.Return.Error, nil
	}
}

func downloadAgentBinary(endpoint, credential, buildID, osName, archName, outPath string) error {
	url := fmt.Sprintf("%s/api/worker/agent/download/%s?os=%s&arch=%s", endpoint, buildID, osName, archName)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+credential)

	resp, err := updateHTTPClient(5 * time.Minute).Do(req)
	if err != nil {
		return fmt.Errorf("download gagal, job manager tidak dapat dihubungi: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("download gagal (HTTP %d): %s", resp.StatusCode, extractReturnMessage(body))
	}

	if dir := filepath.Dir(outPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("gagal membuat direktori output: %w", err)
		}
	}

	tmpPath := outPath + ".download"
	tmp, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("gagal membuat file output: %w", err)
	}

	written, copyErr := io.Copy(tmp, resp.Body)
	closeErr := tmp.Close()
	if copyErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("gagal menulis binary: %w", copyErr)
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("gagal menutup file output: %w", closeErr)
	}
	if written == 0 {
		os.Remove(tmpPath)
		return fmt.Errorf("file binary kosong")
	}

	if err := os.Rename(tmpPath, outPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("gagal memindahkan binary ke %s: %w", outPath, err)
	}

	fmt.Printf("✓ Binary tersimpan (%d bytes)\n", written)
	return nil
}

// extractReturnMessage pulls the human readable message out of the manager response.
func extractReturnMessage(body []byte) string {
	var generic struct {
		Return json.RawMessage `json:"return"`
	}
	if err := json.Unmarshal(body, &generic); err != nil {
		return strings.TrimSpace(string(body))
	}
	var asString string
	if err := json.Unmarshal(generic.Return, &asString); err == nil {
		return asString
	}
	return strings.TrimSpace(string(generic.Return))
}
