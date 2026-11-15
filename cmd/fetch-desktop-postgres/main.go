// Command fetch-desktop-postgres downloads the embedded PostgreSQL build that
// the desktop app bundles, for one target platform, and writes it under the
// file name the sidecar expects:
//
//	embedded-postgres-binaries-<os>-<arch>-<version>.txz
//
// It uses the same Maven artifacts and naming as
// github.com/fergusstrange/embedded-postgres, but unlike the library's own
// download it refuses to continue without a matching published checksum.
//
//	go run ./cmd/fetch-desktop-postgres --goos darwin --goarch arm64 --out dist/desktop/postgres
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha1" //nolint:gosec // Maven publishes SHA-1 for every artifact; SHA-256 is preferred when present.
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"hash"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/wikiSubmission/ws-backend/desktop"
)

const mavenRepo = "https://repo1.maven.org/maven2"

func main() {
	goos := flag.String("goos", runtime.GOOS, "target OS: windows, darwin or linux")
	goarch := flag.String("goarch", runtime.GOARCH, "target architecture: amd64 or arm64")
	out := flag.String("out", "", "output folder")
	flag.Parse()
	if *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*goos, *goarch, *out); err != nil {
		log.Fatal(err)
	}
}

// artifactPlatform mirrors the library's default version strategy for the
// platforms the desktop app ships on.
func artifactPlatform(goos, goarch string) (string, string, error) {
	switch {
	case goos == "windows" && goarch == "amd64",
		goos == "linux" && goarch == "amd64",
		goos == "darwin" && goarch == "amd64":
		return goos, goarch, nil
	case (goos == "darwin" || goos == "linux") && goarch == "arm64":
		return goos, "arm64v8", nil
	default:
		return "", "", fmt.Errorf("unsupported platform %s/%s", goos, goarch)
	}
}

func run(goos, goarch, outDir string) error {
	osName, arch, err := artifactPlatform(goos, goarch)
	if err != nil {
		return err
	}
	version := string(desktop.PostgresVersion)
	artifact := fmt.Sprintf("embedded-postgres-binaries-%s-%s", osName, arch)
	jarURL := fmt.Sprintf("%s/io/zonky/test/postgres/%s/%s/%s-%s.jar", mavenRepo, artifact, version, artifact, version)

	client := &http.Client{Timeout: 5 * time.Minute}
	jar, err := get(client, jarURL)
	if err != nil {
		return err
	}
	if err := verify(client, jarURL, jar); err != nil {
		return err
	}

	zr, err := zip.NewReader(bytes.NewReader(jar), int64(len(jar)))
	if err != nil {
		return fmt.Errorf("open %s: %w", jarURL, err)
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !strings.HasSuffix(f.Name, ".txz") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return err
		}
		// The name the sidecar's embedded-postgres cache lookup expects.
		dest := filepath.Join(outDir, fmt.Sprintf("%s-%s.txz", artifact, version))
		w, err := os.Create(dest)
		if err != nil {
			return err
		}
		n, err := io.Copy(w, rc)
		if cerr := w.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		fmt.Printf("%s (%.1f MB)\n", dest, float64(n)/(1<<20))
		return nil
	}
	return fmt.Errorf("no .txz inside %s", jarURL)
}

// verify checks the jar against the checksum Maven publishes next to it,
// preferring SHA-256. A missing or mismatching checksum is an error.
func verify(client *http.Client, jarURL string, jar []byte) error {
	for _, c := range []struct {
		ext string
		h   func() hash.Hash
	}{{".sha256", sha256.New}, {".sha1", sha1.New}} {
		want, err := get(client, jarURL+c.ext)
		if err != nil {
			continue
		}
		h := c.h()
		h.Write(jar)
		got := hex.EncodeToString(h.Sum(nil))
		fields := strings.Fields(string(want))
		if len(fields) == 0 || !strings.EqualFold(fields[0], got) {
			return fmt.Errorf("checksum mismatch for %s (%s)", jarURL, c.ext)
		}
		fmt.Printf("verified %s%s\n", filepath.Base(jarURL), c.ext)
		return nil
	}
	return errors.New("no published checksum found for " + jarURL)
}

func get(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}
