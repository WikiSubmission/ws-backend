package main

// End-to-end test of `ws-backend --desktop`, driven the way the Electron shell
// drives it: config on stdin, readiness line on stdout, shutdown by closing
// stdin. It builds the real binary and starts a real embedded Postgres, so it
// is opt-in:
//
//	WS_DESKTOP_E2E_PG_ARCHIVE  folder holding embedded-postgres-binaries-<os>-<arch>-18.3.0.txz (required)
//	WS_DESKTOP_E2E_SNAPSHOT    snapshot folder from cmd/build-desktop-snapshot (optional; enables content checks)
//	WS_DESKTOP_E2E_WORKDIR     where to put the build and data (optional; defaults to a temp dir)
//
//	go test ./api -run TestDesktopEndToEnd -v -timeout 15m

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

type desktopProc struct {
	t     *testing.T
	cmd   *exec.Cmd
	stdin io.WriteCloser
	port  int
	token string
}

func TestDesktopEndToEnd(t *testing.T) {
	archive := os.Getenv("WS_DESKTOP_E2E_PG_ARCHIVE")
	if archive == "" {
		t.Skip("set WS_DESKTOP_E2E_PG_ARCHIVE to run the desktop end-to-end test")
	}
	snapshot := os.Getenv("WS_DESKTOP_E2E_SNAPSHOT")
	work := os.Getenv("WS_DESKTOP_E2E_WORKDIR")
	if work == "" {
		work = t.TempDir()
	} else {
		work = filepath.Join(work, fmt.Sprintf("e2e-%d", time.Now().UnixNano()))
		t.Cleanup(func() { os.RemoveAll(work) })
	}

	bin := filepath.Join(work, "ws-backend-desktop")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	// A non-ASCII folder exercises the Windows path fallback.
	data := filepath.Join(work, "data Müller")
	logPath := filepath.Join(work, "sidecar.log")
	cfg := map[string]string{"dataDir": data, "pgArchiveDir": archive, "snapshotDir": snapshot}

	p := startDesktop(t, bin, cfg, logPath)

	t.Run("routing and auth", func(t *testing.T) {
		p.expect(t, "GET", "/health", nil, 200)
		p.expect(t, "GET", "/api/v1/users/me", nil, 200)
		p.expectNoToken(t, "/api/v1/users/me", 401)
		for _, path := range []string{
			"/metrics", "/api/v1/bible/books", "/api/v1/music/tracks",
			"/api/v1/editorial/public/articles", "/api/v1/admin/offline/files",
			"/api/v1/me/export", "/api/v1/auth/mobile/exchange",
		} {
			p.expect(t, "GET", path, nil, 404)
		}
		req, _ := http.NewRequest("GET", p.url("/health"), nil)
		req.Host = "evil.example"
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMisdirectedRequest {
			t.Errorf("foreign Host: got %d, want 421", resp.StatusCode)
		}
	})

	if snapshot != "" {
		t.Run("content", func(t *testing.T) {
			var chapters []any
			p.getJSON(t, "/api/v1/chapters", &chapters)
			if len(chapters) != 114 {
				t.Errorf("chapters: got %d, want 114", len(chapters))
			}
			body := p.expect(t, "GET", "/api/v1/quran?chapter_number_start=1&langs=en&langs=ar", nil, 200)
			if !bytes.Contains(body, []byte("Most Gracious")) || !bytes.Contains(body, []byte("بِسْمِ")) {
				t.Error("chapter 1 is missing English or Arabic text")
			}
			var search struct {
				Info struct {
					Total int `json:"total"`
				} `json:"info"`
			}
			p.getJSON(t, "/api/v1/search?q=mercy&langs=en", &search)
			if search.Info.Total == 0 {
				t.Error("search for mercy returned nothing")
			}
			p.expect(t, "GET", "/api/v1/topic-index?letter=A", nil, 200)
			p.expect(t, "GET", "/api/v1/roots", nil, 200)
			var apps struct {
				Data []any `json:"data"`
			}
			p.getJSON(t, "/api/v1/editorial/public/appendices?language=en", &apps)
			if len(apps.Data) == 0 {
				t.Error("no appendices")
			}
		})
	}

	var backup []byte
	t.Run("backup and restore", func(t *testing.T) {
		p.expect(t, "PUT", "/api/v1/me/notes", map[string]any{"scripture": "quran", "verse_key": "2:255", "content": "kept by restore"}, 200)
		var cat struct {
			Data struct {
				ID int `json:"id"`
			} `json:"data"`
		}
		p.sendJSON(t, "POST", "/api/v1/me/bookmark-categories", map[string]any{"name": "Favorites", "color": "amber"}, &cat)
		p.expect(t, "POST", "/api/v1/me/bookmark-entries", map[string]any{"category_id": cat.Data.ID, "scripture": "quran", "verse_key": "1:1"}, 0)

		backup = p.expect(t, "GET", "/api/v1/desktop/backup", nil, 200)
		if !bytes.Contains(backup, []byte("kept by restore")) || !bytes.Contains(backup, []byte(`"1:1"`)) {
			t.Fatalf("backup is missing the note or bookmark: %.300s", backup)
		}

		// Change things after the backup.
		p.expect(t, "PUT", "/api/v1/me/notes", map[string]any{"scripture": "quran", "verse_key": "3:1", "content": "added after backup"}, 200)
		p.expect(t, "DELETE", fmt.Sprintf("/api/v1/me/bookmark-categories/%d", cat.Data.ID), nil, 0)

		// A bad file is rejected and changes nothing.
		p.expectRaw(t, "POST", "/api/v1/desktop/restore", []byte(`{"kind":"something-else","format":1,"tables":{}}`), 400)
		if !bytes.Contains(p.expect(t, "GET", "/api/v1/me/notes", nil, 200), []byte("added after backup")) {
			t.Fatal("a rejected restore changed data")
		}

		p.expectRaw(t, "POST", "/api/v1/desktop/restore", backup, 200)
		notes := p.expect(t, "GET", "/api/v1/me/notes", nil, 200)
		if !bytes.Contains(notes, []byte("kept by restore")) || bytes.Contains(notes, []byte("added after backup")) {
			t.Errorf("notes after restore are wrong: %.300s", notes)
		}
		entries := p.expect(t, "GET", fmt.Sprintf("/api/v1/me/bookmark-categories/%d/entries", cat.Data.ID), nil, 200)
		if !bytes.Contains(entries, []byte(`"1:1"`)) {
			t.Errorf("bookmark entry not restored under its folder: %.300s", entries)
		}
		// Ids are restored as-is, so new rows must not collide with them.
		p.expect(t, "PUT", "/api/v1/me/notes", map[string]any{"scripture": "quran", "verse_key": "4:1", "content": "new after restore"}, 200)
	})

	t.Run("restart keeps data", func(t *testing.T) {
		p.shutdown(t)
		p = startDesktop(t, bin, cfg, logPath)
		if !bytes.Contains(p.expect(t, "GET", "/api/v1/me/notes", nil, 200), []byte("kept by restore")) {
			t.Error("note lost across a clean restart")
		}
	})

	t.Run("recovers after a crash", func(t *testing.T) {
		// Kill the sidecar outright: its Postgres is orphaned, and the next
		// launch must stop it and start cleanly.
		_ = p.cmd.Process.Kill()
		_, _ = p.cmd.Process.Wait()
		time.Sleep(500 * time.Millisecond)
		p = startDesktop(t, bin, cfg, logPath)
		if !bytes.Contains(p.expect(t, "GET", "/api/v1/me/notes", nil, 200), []byte("kept by restore")) {
			t.Error("note lost across a crash")
		}
	})

	p.shutdown(t)
}

func startDesktop(t *testing.T, bin string, cfg map[string]string, logPath string) *desktopProc {
	t.Helper()
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "--desktop")
	cmd.Stderr = logFile
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, _ := json.Marshal(cfg)
	if _, err := stdin.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}

	type ready struct {
		Ready bool   `json:"ready"`
		Port  int    `json:"port"`
		Token string `json:"token"`
	}
	got := make(chan ready, 1)
	go func() {
		var r ready
		if s := bufio.NewScanner(stdout); s.Scan() {
			_ = json.Unmarshal(s.Bytes(), &r)
		}
		got <- r
		_, _ = io.Copy(io.Discard, stdout)
	}()
	select {
	case r := <-got:
		if !r.Ready {
			_ = cmd.Process.Kill()
			t.Fatalf("sidecar did not become ready; see %s", logPath)
		}
		return &desktopProc{t: t, cmd: cmd, stdin: stdin, port: r.Port, token: r.Token}
	case <-time.After(5 * time.Minute):
		_ = cmd.Process.Kill()
		t.Fatalf("sidecar startup timed out; see %s", logPath)
	}
	return nil
}

func (p *desktopProc) url(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", p.port, path)
}

func (p *desktopProc) do(t *testing.T, method, path string, body []byte, token bool) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, p.url(path), r)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// expect sends a JSON body (or none) and checks the status; want 0 accepts
// any 2xx.
func (p *desktopProc) expect(t *testing.T, method, path string, body any, want int) []byte {
	t.Helper()
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	return p.expectRaw(t, method, path, raw, want)
}

func (p *desktopProc) expectRaw(t *testing.T, method, path string, body []byte, want int) []byte {
	t.Helper()
	code, b := p.do(t, method, path, body, true)
	if (want == 0 && (code < 200 || code > 299)) || (want != 0 && code != want) {
		t.Errorf("%s %s: got %d, want %d: %.300s", method, path, code, want, b)
	}
	return b
}

func (p *desktopProc) expectNoToken(t *testing.T, path string, want int) {
	t.Helper()
	if code, b := p.do(t, "GET", path, nil, false); code != want {
		t.Errorf("GET %s without token: got %d, want %d: %.200s", path, code, want, b)
	}
}

func (p *desktopProc) getJSON(t *testing.T, path string, out any) {
	t.Helper()
	if err := json.Unmarshal(p.expect(t, "GET", path, nil, 200), out); err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
}

func (p *desktopProc) sendJSON(t *testing.T, method, path string, body, out any) {
	t.Helper()
	if err := json.Unmarshal(p.expect(t, method, path, body, 0), out); err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
}

func (p *desktopProc) shutdown(t *testing.T) {
	t.Helper()
	_ = p.stdin.Close()
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("sidecar exited with %v", err)
		}
	case <-time.After(30 * time.Second):
		_ = p.cmd.Process.Kill()
		t.Error("sidecar did not exit within 30s of stdin closing")
	}
}
