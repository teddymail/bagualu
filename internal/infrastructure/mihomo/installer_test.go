package mihomo

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestChooseAssetPrefersCompatibleLinuxBinary(t *testing.T) {
	asset, err := chooseAsset([]githubAsset{
		{Name: "mihomo-linux-amd64-v1.gz", BrowserDownloadURL: "https://example.invalid/v1"},
		{Name: "mihomo-linux-amd64-v3.gz", BrowserDownloadURL: "https://example.invalid/v3"},
		{Name: "mihomo-linux-amd64-compatible.gz", BrowserDownloadURL: "https://example.invalid/compatible"},
		{Name: "mihomo-linux-amd64-compatible.deb", BrowserDownloadURL: "https://example.invalid/deb"},
	}, "linux", "amd64")
	if err != nil {
		t.Fatalf("choose asset: %v", err)
	}
	if asset.Name != "mihomo-linux-amd64-compatible.gz" {
		t.Fatalf("asset = %q", asset.Name)
	}
}

func TestSplitRepository(t *testing.T) {
	owner, name, err := splitRepository("MetaCubeX/mihomo")
	if err != nil || owner != "MetaCubeX" || name != "mihomo" {
		t.Fatalf("split repository = %q/%q, %v", owner, name, err)
	}
	if _, _, err := splitRepository("MetaCubeX/mihomo/releases"); err == nil {
		t.Fatal("expected invalid repository error")
	}
}

func TestInstallerDownloadsVerifiesAndActivatesBinary(t *testing.T) {
	binary := []byte{0x7f, 'E', 'L', 'F', 1, 2, 3, 4}
	var archive bytesBuffer
	writer := gzip.NewWriter(&archive)
	if _, err := writer.Write(binary); err != nil {
		t.Fatalf("compress binary: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	sum := sha256.Sum256(archive.Bytes())
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/test/repo/releases/latest":
			_ = json.NewEncoder(w).Encode(githubRelease{TagName: "v1.2.3", Assets: []githubAsset{{
				Name: "mihomo-linux-amd64-compatible.gz", BrowserDownloadURL: serverURL(r, "/download"), Digest: "sha256:" + hex.EncodeToString(sum[:]), Size: int64(archive.Len()),
			}}})
		case "/download":
			w.Write(archive.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "mihomo")
	installer := NewInstaller(destination, "test/repo", "latest")
	installer.apiBaseURL = server.URL
	installer.client = server.Client()
	installer.goos = "linux"
	installer.goarch = "amd64"
	result, err := installer.InstallLatest(context.Background())
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !result.Verified || result.Version != "v1.2.3" {
		t.Fatalf("unexpected result: %+v", result)
	}
	installed, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read installed binary: %v", err)
	}
	if string(installed) != string(binary) {
		t.Fatalf("installed binary = %v", installed)
	}
	info, err := os.Stat(destination)
	if err != nil || info.Mode()&0111 == 0 {
		t.Fatalf("installed binary is not executable: %v", err)
	}
}

func TestInstallerInstallsUploadedBinary(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "mihomo")
	installer := NewInstaller(destination, "MetaCubeX/mihomo", "latest")
	installer.goos = "linux"
	installer.goarch = "amd64"
	result, err := installer.InstallFile(context.Background(), bytes.NewReader([]byte{0x7f, 'E', 'L', 'F', 1}), "mihomo-upload")
	if err != nil {
		t.Fatalf("install upload: %v", err)
	}
	if result.Version != "uploaded" || result.Asset != "mihomo-upload" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func serverURL(request *http.Request, path string) string {
	return "https://" + request.Host + path
}

type bytesBuffer struct{ data []byte }

func (b *bytesBuffer) Write(value []byte) (int, error) {
	b.data = append(b.data, value...)
	return len(value), nil
}

func (b *bytesBuffer) Bytes() []byte { return b.data }
func (b *bytesBuffer) Len() int      { return len(b.data) }
