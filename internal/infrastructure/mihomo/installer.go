package mihomo

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

const (
	defaultMihomoRepository = "MetaCubeX/mihomo"
	defaultGitHubAPI        = "https://api.github.com"
	maxReleaseSize          = 128 << 20
	maxBinarySize           = 256 << 20
)

type Installer struct {
	binaryPath string
	repository string
	version    string
	apiBaseURL string
	client     *http.Client
	goos       string
	goarch     string
	mu         sync.Mutex
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
	Size               int64  `json:"size"`
}

func NewInstaller(binaryPath, repository, version string) *Installer {
	if repository == "" {
		repository = defaultMihomoRepository
	}
	if version == "" {
		version = "latest"
	}
	return &Installer{
		binaryPath: binaryPath,
		repository: repository,
		version:    version,
		apiBaseURL: defaultGitHubAPI,
		client:     &http.Client{Timeout: 10 * time.Minute},
		goos:       runtime.GOOS,
		goarch:     runtime.GOARCH,
	}
}

func (i *Installer) Status(ctx context.Context) domain.CoreInstallStatus {
	status := domain.CoreInstallStatus{
		Path:         i.binaryPath,
		Architecture: i.goos + "/" + i.goarch,
		Source:       i.repository,
	}
	info, err := os.Stat(i.binaryPath)
	if err != nil {
		if !os.IsNotExist(err) {
			status.Error = err.Error()
		}
		return status
	}
	if info.IsDir() || info.Mode()&0111 == 0 || info.Size() == 0 {
		status.Error = "mihomo binary is not executable"
		return status
	}
	status.Installed = true
	status.Version = detectVersion(ctx, i.binaryPath)
	return status
}

func (i *Installer) InstallLatest(ctx context.Context) (domain.CoreInstallResult, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	release, err := i.fetchRelease(ctx)
	if err != nil {
		return domain.CoreInstallResult{}, err
	}
	asset, err := chooseAsset(release.Assets, i.goos, i.goarch)
	if err != nil {
		return domain.CoreInstallResult{}, err
	}
	if asset.Size > maxReleaseSize {
		return domain.CoreInstallResult{}, fmt.Errorf("mihomo release asset is too large")
	}
	archive, err := i.download(ctx, asset.BrowserDownloadURL)
	if err != nil {
		return domain.CoreInstallResult{}, err
	}
	verified, digest, err := verifyDigest(archive, asset.Digest)
	if err != nil {
		return domain.CoreInstallResult{}, err
	}
	if err := i.installBinary(archive, isGzipArchive(archive)); err != nil {
		return domain.CoreInstallResult{}, err
	}
	return domain.CoreInstallResult{
		Version:  release.TagName,
		Asset:    asset.Name,
		Path:     i.binaryPath,
		Verified: verified,
		SHA256:   digest,
	}, nil
}

func (i *Installer) InstallFile(ctx context.Context, source io.Reader, name string) (domain.CoreInstallResult, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	archive, err := io.ReadAll(io.LimitReader(source, maxReleaseSize+1))
	if err != nil {
		return domain.CoreInstallResult{}, fmt.Errorf("read uploaded Mihomo file: %w", err)
	}
	if len(archive) == 0 || len(archive) > maxReleaseSize {
		return domain.CoreInstallResult{}, fmt.Errorf("uploaded Mihomo file is empty or too large")
	}
	if err := i.installBinary(archive, isGzipArchive(archive)); err != nil {
		return domain.CoreInstallResult{}, err
	}
	sum := sha256.Sum256(archive)
	return domain.CoreInstallResult{Version: "uploaded", Asset: name, Path: i.binaryPath, SHA256: hex.EncodeToString(sum[:])}, nil
}

func ValidateArchive(archive []byte) error {
	var source io.Reader = bytes.NewReader(archive)
	if isGzipArchive(archive) {
		reader, err := gzip.NewReader(bytes.NewReader(archive))
		if err != nil {
			return fmt.Errorf("open mihomo archive: %w", err)
		}
		defer reader.Close()
		source = reader
	}
	binary, err := io.ReadAll(io.LimitReader(source, maxBinarySize+1))
	if err != nil {
		return fmt.Errorf("extract mihomo binary: %w", err)
	}
	if len(binary) == 0 || len(binary) > maxBinarySize {
		return fmt.Errorf("mihomo binary is empty or too large")
	}
	if len(binary) < 4 || string(binary[:4]) != "\x7fELF" {
		return fmt.Errorf("downloaded mihomo file is not an ELF executable")
	}
	return nil
}

func (i *Installer) fetchRelease(ctx context.Context) (githubRelease, error) {
	owner, name, err := splitRepository(i.repository)
	if err != nil {
		return githubRelease{}, err
	}
	repositoryPath := url.PathEscape(owner) + "/" + url.PathEscape(name)
	endpoint := strings.TrimRight(i.apiBaseURL, "/") + "/repos/" + repositoryPath + "/releases/latest"
	if i.version != "latest" {
		endpoint = strings.TrimRight(i.apiBaseURL, "/") + "/repos/" + repositoryPath + "/releases/tags/" + url.PathEscape(i.version)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return githubRelease{}, fmt.Errorf("build mihomo release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "bagualu-mihomo-installer")
	resp, err := i.client.Do(req)
	if err != nil {
		return githubRelease{}, fmt.Errorf("download mihomo release metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return githubRelease{}, fmt.Errorf("mihomo release metadata returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		return githubRelease{}, fmt.Errorf("decode mihomo release metadata: %w", err)
	}
	if release.TagName == "" {
		return githubRelease{}, fmt.Errorf("mihomo release metadata has no version")
	}
	return release, nil
}

func splitRepository(repository string) (string, string, error) {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("mihomo repository must use owner/name format")
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func (i *Installer) download(ctx context.Context, location string) ([]byte, error) {
	parsed, err := url.Parse(location)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("mihomo release URL must use HTTPS")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build mihomo download request: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "bagualu-mihomo-installer")
	resp, err := i.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download mihomo binary: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mihomo binary download returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseSize+1))
	if err != nil {
		return nil, fmt.Errorf("read mihomo binary: %w", err)
	}
	if len(data) == 0 || len(data) > maxReleaseSize {
		return nil, fmt.Errorf("mihomo binary download is empty or too large")
	}
	return data, nil
}

func (i *Installer) installBinary(archive []byte, compressed bool) error {
	dir := filepath.Dir(i.binaryPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create mihomo binary directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".mihomo-install-*")
	if err != nil {
		return fmt.Errorf("create mihomo install file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	defer temp.Close()

	var source io.Reader = bytes.NewReader(archive)
	if compressed {
		reader, gzipErr := gzip.NewReader(bytes.NewReader(archive))
		if gzipErr != nil {
			return fmt.Errorf("open mihomo archive: %w", gzipErr)
		}
		defer reader.Close()
		source = reader
	}
	written, err := io.Copy(temp, io.LimitReader(source, maxBinarySize+1))
	if err != nil {
		return fmt.Errorf("extract mihomo binary: %w", err)
	}
	if written == 0 || written > maxBinarySize {
		return fmt.Errorf("mihomo binary is empty or too large")
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync mihomo binary: %w", err)
	}
	if _, err := temp.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("check mihomo binary: %w", err)
	}
	header := make([]byte, 4)
	if _, err := io.ReadFull(temp, header); err != nil || string(header) != "\x7fELF" {
		return fmt.Errorf("downloaded mihomo file is not an ELF executable")
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close mihomo binary: %w", err)
	}
	if err := os.Chmod(tempPath, 0755); err != nil {
		return fmt.Errorf("make mihomo executable: %w", err)
	}
	if err := os.Rename(tempPath, i.binaryPath); err != nil {
		return fmt.Errorf("activate mihomo binary: %w", err)
	}
	return nil
}

func verifyDigest(data []byte, expected string) (bool, string, error) {
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	if expected == "" {
		return false, digest, nil
	}
	value := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(expected)), "sha256:")
	if value != digest {
		return false, digest, fmt.Errorf("mihomo release checksum mismatch")
	}
	return true, digest, nil
}

func isGzipArchive(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
}

func chooseAsset(assets []githubAsset, goos, goarch string) (githubAsset, error) {
	type candidate struct {
		asset githubAsset
		score int
	}
	target := architectureToken(goarch)
	if goos != "linux" || target == "" {
		return githubAsset{}, fmt.Errorf("mihomo installer does not support %s/%s", goos, goarch)
	}
	candidates := make([]candidate, 0)
	for _, asset := range assets {
		name := strings.ToLower(asset.Name)
		if asset.BrowserDownloadURL == "" || !strings.Contains(name, "linux") || !strings.Contains(name, target) || isPackageOrChecksum(name) {
			continue
		}
		score := 10
		if strings.Contains(name, "compatible") {
			score += 150
		}
		if strings.Contains(name, target+"-v1") {
			score += 100
		}
		if strings.Contains(name, target+"-v2") {
			score += 50
		}
		if strings.Contains(name, "v3") {
			score -= 20
		}
		if strings.HasSuffix(name, ".gz") {
			score += 5
		}
		candidates = append(candidates, candidate{asset: asset, score: score})
	}
	if len(candidates) == 0 {
		return githubAsset{}, fmt.Errorf("no compatible Mihomo asset for %s/%s", goos, goarch)
	}
	sort.SliceStable(candidates, func(left, right int) bool { return candidates[left].score > candidates[right].score })
	return candidates[0].asset, nil
}

func architectureToken(goarch string) string {
	switch goarch {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "386":
		return "386"
	case "mipsle":
		return "mipsle"
	default:
		return ""
	}
}

func isPackageOrChecksum(name string) bool {
	for _, suffix := range []string{".deb", ".rpm", ".pkg", ".zip", ".tar.gz", ".sha256", ".sha256sum", ".sig", ".dgst"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return strings.Contains(name, "checksum") || strings.Contains(name, "checksums")
}

func detectVersion(ctx context.Context, binaryPath string) string {
	commandCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for _, args := range [][]string{{"-v"}, {"version"}} {
		output, err := runVersionCommand(commandCtx, binaryPath, args...)
		if err == nil && strings.TrimSpace(output) != "" {
			return strings.TrimSpace(strings.SplitN(output, "\n", 2)[0])
		}
	}
	return ""
}

func runVersionCommand(ctx context.Context, binaryPath string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, binaryPath, args...)
	output, err := command.Output()
	return string(output), err
}
