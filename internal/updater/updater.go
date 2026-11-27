package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Version is injected at build time via -ldflags.
var Version = "dev"

const (
	Repo     = "GongJS/agym"
	checkTTL = 24 * time.Hour
)

type ReleaseInfo struct {
	Version   string `json:"version"`
	URL       string `json:"url"`
	CheckedAt int64  `json:"checkedAt"`
}

func cachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "agy-tools", "update_check.json")
}

// IsNewer reports whether latest is strictly newer than current using semver.
// Development builds ("dev") never trigger automatic update prompts.
func IsNewer(latest, current string) bool {
	cur := strings.TrimSpace(current)
	if cur == "" || cur == "dev" {
		return false
	}
	lParts, lOK := parseSemver(latest)
	cParts, cOK := parseSemver(cur)
	if !lOK || !cOK {
		return false
	}
	for i := 0; i < 3; i++ {
		if lParts[i] != cParts[i] {
			return lParts[i] > cParts[i]
		}
	}
	return false
}

func parseSemver(raw string) ([3]int, bool) {
	var out [3]int
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "v")
	if idx := strings.IndexAny(s, "-+"); idx >= 0 {
		s = s[:idx]
	}
	if s == "" {
		return out, false
	}
	segs := strings.Split(s, ".")
	if len(segs) < 2 || len(segs) > 3 {
		return out, false
	}
	for i, seg := range segs {
		n, err := strconv.Atoi(seg)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// CheckLatest returns the latest GitHub release info, using a 24h cache unless force is true.
func CheckLatest(ctx context.Context, force bool) (ReleaseInfo, error) {
	cPath := cachePath()
	if !force && cPath != "" {
		if raw, err := os.ReadFile(cPath); err == nil {
			var cached ReleaseInfo
			if json.Unmarshal(raw, &cached) == nil && cached.Version != "" {
				if time.Since(time.Unix(cached.CheckedAt, 0)) < checkTTL {
					return cached, nil
				}
			}
		}
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", Repo)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return ReleaseInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "agym/"+Version)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ReleaseInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ReleaseInfo{}, fmt.Errorf("github releases API returned %s", resp.Status)
	}

	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ReleaseInfo{}, err
	}
	if strings.TrimSpace(payload.TagName) == "" {
		return ReleaseInfo{}, errors.New("empty tag_name in latest release")
	}

	info := ReleaseInfo{
		Version:   strings.TrimSpace(payload.TagName),
		URL:       strings.TrimSpace(payload.HTMLURL),
		CheckedAt: time.Now().Unix(),
	}
	if cPath != "" {
		if data, err := json.Marshal(info); err == nil {
			_ = os.MkdirAll(filepath.Dir(cPath), 0o700)
			_ = os.WriteFile(cPath, data, 0o600)
		}
	}
	return info, nil
}

// SelfUpdate downloads, verifies SHA-256, and atomically replaces the running executable.
func SelfUpdate(ctx context.Context) (string, error) {
	info, err := CheckLatest(ctx, true)
	if err != nil {
		return "", err
	}
	if Version != "dev" && !IsNewer(info.Version, Version) {
		return info.Version, nil
	}

	archiveName, binName, err := releaseAssetNames(info.Version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}

	baseURL := fmt.Sprintf("https://github.com/%s/releases/download/%s", Repo, info.Version)
	tmpDir, err := os.MkdirTemp("", "agym-update-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, archiveName)
	if err := downloadFile(ctx, baseURL+"/"+archiveName, archivePath); err != nil {
		return "", fmt.Errorf("download archive: %w", err)
	}

	checksumsPath := filepath.Join(tmpDir, "checksums.txt")
	if err := downloadFile(ctx, baseURL+"/checksums.txt", checksumsPath); err != nil {
		return "", fmt.Errorf("download checksums: %w", err)
	}
	if err := verifyChecksum(archivePath, archiveName, checksumsPath); err != nil {
		return "", err
	}

	extractedPath := filepath.Join(tmpDir, binName)
	if strings.HasSuffix(archiveName, ".tar.gz") {
		err = extractTarGzBinary(archivePath, binName, extractedPath)
	} else {
		err = extractZipBinary(archivePath, binName, extractedPath)
	}
	if err != nil {
		return "", err
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	if err := replaceExecutable(exePath, extractedPath); err != nil {
		return "", err
	}
	return info.Version, nil
}

func releaseAssetNames(tag, goos, goarch string) (archive string, bin string, err error) {
	verNoV := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if verNoV == "" {
		return "", "", errors.New("invalid release tag")
	}
	switch goos {
	case "darwin":
		if goarch != "arm64" && goarch != "amd64" {
			return "", "", fmt.Errorf("unsupported macOS architecture: %s", goarch)
		}
		return fmt.Sprintf("agym_%s_darwin_%s.tar.gz", verNoV, goarch), "agym", nil
	case "windows":
		if goarch != "amd64" {
			return "", "", fmt.Errorf("unsupported Windows architecture: %s", goarch)
		}
		return fmt.Sprintf("agym_%s_windows_%s.zip", verNoV, goarch), "agym.exe", nil
	default:
		return "", "", fmt.Errorf("unsupported OS for self-update: %s", goos)
	}
}

func downloadFile(ctx context.Context, url, dst string) error {
	reqCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "agym/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func verifyChecksum(archivePath, archiveName, checksumsPath string) error {
	raw, err := os.ReadFile(checksumsPath)
	if err != nil {
		return err
	}
	expected := ""
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[1], "*") == archiveName {
			expected = strings.ToLower(fields[0])
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("checksum entry not found for %s", archiveName)
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", archiveName, actual, expected)
	}
	return nil
}

func extractTarGzBinary(archivePath, binName, dst string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if filepath.Base(hdr.Name) == binName && hdr.Typeflag == tar.TypeReg {
			out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		}
	}
	return fmt.Errorf("binary %s not found in archive", binName)
}

func extractZipBinary(archivePath, binName, dst string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if filepath.Base(f.Name) == binName && !f.FileInfo().IsDir() {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				_ = rc.Close()
				return err
			}
			_, copyErr := io.Copy(out, rc)
			closeErr := out.Close()
			_ = rc.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		}
	}
	return fmt.Errorf("binary %s not found in zip archive", binName)
}

func replaceExecutable(exePath, newBinPath string) error {
	dir := filepath.Dir(exePath)
	staged, err := os.CreateTemp(dir, ".agym-update-*")
	if err != nil {
		return fmt.Errorf("stage binary in %s: %w", dir, err)
	}
	stagedPath := staged.Name()
	src, err := os.Open(newBinPath)
	if err != nil {
		_ = staged.Close()
		_ = os.Remove(stagedPath)
		return err
	}
	_, copyErr := io.Copy(staged, src)
	_ = src.Close()
	chmodErr := staged.Chmod(0o755)
	closeErr := staged.Close()
	if copyErr != nil || chmodErr != nil || closeErr != nil {
		_ = os.Remove(stagedPath)
		return errors.Join(copyErr, chmodErr, closeErr)
	}

	if runtime.GOOS == "windows" {
		oldPath := exePath + ".old"
		_ = os.Remove(oldPath)
		if err := os.Rename(exePath, oldPath); err != nil {
			_ = os.Remove(stagedPath)
			return err
		}
	}
	if err := os.Rename(stagedPath, exePath); err != nil {
		_ = os.Remove(stagedPath)
		return err
	}
	return nil
}
