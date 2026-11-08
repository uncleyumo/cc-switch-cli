// Package update implements `ccsc upgrade`: check GitHub Releases, download
// the asset for this platform, verify its checksum and replace the running
// binary with a dated backup beside it.
package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// UpdateRepo is the public repository releases are published to.
const UpdateRepo = "uncleyumo/cc-switch-cli"

// Release is the subset of the GitHub release object ccsc needs.
type Release struct {
	Tag     string
	Name    string
	Body    string
	HTMLURL string
	Assets  map[string]string // asset name -> download URL
}

// AssetName is the archive this platform needs.
func AssetName() string {
	return fmt.Sprintf("ccsc_%s_%s.zip", runtime.GOOS, runtime.GOARCH)
}

// Latest fetches the newest release.
func Latest(ctx context.Context) (*Release, error) {
	url := "https://api.github.com/repos/" + UpdateRepo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ccsc")
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach GitHub: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub returned %s", resp.Status)
	}
	var raw struct {
		TagName string `json:"tag_name"`
		Name    string `json:"name"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("cannot parse the GitHub response: %w", err)
	}
	rel := &Release{Tag: raw.TagName, Name: raw.Name, Body: raw.Body, HTMLURL: raw.HTMLURL, Assets: map[string]string{}}
	for _, a := range raw.Assets {
		rel.Assets[a.Name] = a.URL
	}
	return rel, nil
}

// ChecksumURL is the checksums.txt asset of the release, or "".
func ChecksumURL(rel *Release) string {
	if rel == nil {
		return ""
	}
	return rel.Assets["checksums.txt"]
}

// Download fetches a URL into memory, refusing anything larger than maxBytes.
func Download(ctx context.Context, url string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ccsc")
	if token := githubToken(); token != "" && strings.Contains(url, "github.com") {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("download is larger than the %d byte limit", maxBytes)
	}
	return data, nil
}

// VerifySHA256 checks data against the line for assetName in a checksums file.
func VerifySHA256(data, checksums []byte, assetName string) error {
	sum := sha256.Sum256(data)
	want := hex.EncodeToString(sum[:])
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name != assetName {
			continue
		}
		if !strings.EqualFold(fields[0], want) {
			return fmt.Errorf("checksum mismatch for %s", assetName)
		}
		return nil
	}
	return fmt.Errorf("checksums.txt has no entry for %s", assetName)
}

// Extract returns the named member of a zip archive.
func Extract(zipBytes []byte, wantName string) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("cannot read the archive: %w", err)
	}
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if filepath.Base(f.Name) != wantName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("%s is not in the archive", wantName)
}

// BinaryName is the executable name inside the archive.
func BinaryName() string {
	if runtime.GOOS == "windows" {
		return "ccsc.exe"
	}
	return "ccsc"
}

// Replace swaps the running binary for newBinary and returns the backup path.
//
// The rename comes first on purpose: Windows refuses to overwrite a running
// executable, and on every platform the rename keeps a usable copy on disk
// until the new file is completely written.
func Replace(execPath string, newBinary []byte) (string, error) {
	resolved, err := filepath.EvalSymlinks(execPath)
	if err == nil {
		execPath = resolved
	}
	info, err := os.Stat(execPath)
	if err != nil {
		return "", fmt.Errorf("cannot stat %s: %w", execPath, err)
	}

	backup := execPath + ".bak." + time.Now().Format("20060102")
	if err := os.Rename(execPath, backup); err != nil {
		return "", fmt.Errorf("cannot move the current binary aside: %w", err)
	}

	if err := os.WriteFile(execPath, newBinary, info.Mode().Perm()); err != nil {
		// Put the old binary back rather than leaving the user with nothing.
		_ = os.Rename(backup, execPath)
		return "", fmt.Errorf("cannot write the new binary: %w", err)
	}
	pruneBackups(execPath)
	return backup, nil
}

// SelfCheck runs the freshly installed binary and requires a clean exit.
func SelfCheck(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	exe, err := os.Executable()
	if err != nil {
		return nil // Nothing to verify against; do not fail the upgrade.
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "--version")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("the new binary failed to run: %w", err)
	}
	return nil
}

func pruneBackups(execPath string) {
	dir := filepath.Dir(execPath)
	base := filepath.Base(execPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var backups []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), base+".bak.") && !e.IsDir() {
			backups = append(backups, e.Name())
		}
	}
	if len(backups) <= 1 {
		return
	}
	sort.Strings(backups)
	for _, name := range backups[:len(backups)-1] {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

func githubToken() string {
	for _, key := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}
