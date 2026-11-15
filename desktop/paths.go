// Package desktop runs ws-backend as a local sidecar for the offline desktop
// app: an embedded PostgreSQL, a fixed local user, and a server bound to the
// loopback interface.
package desktop

import (
	"fmt"
	"os"
	"path/filepath"
)

// Layout is the on-disk arrangement of one desktop install. Everything lives
// under Root, which is guaranteed to be an ASCII-only path (see ResolveRoot).
type Layout struct {
	Root string
	// Binaries holds the extracted Postgres distribution. It is extracted once
	// per Postgres version and reused on later launches.
	Binaries string
	// Runtime is scratch space the embedded-postgres library deletes and
	// recreates on every start, so nothing durable may live here.
	Runtime string
	// Data is the Postgres cluster: content tables plus all user data.
	Data string
}

// NewLayout derives the folders for a resolved root.
func NewLayout(root, pgVersion string) Layout {
	return Layout{
		Root:     root,
		Binaries: filepath.Join(root, "pg-"+pgVersion),
		Runtime:  filepath.Join(root, "pg-runtime"),
		Data:     filepath.Join(root, "pgdata"),
	}
}

// ResolveRoot returns a directory equivalent to preferred whose path is
// ASCII-only, creating it if needed.
//
// On Windows, initdb writes the data directory path into the cluster in the
// system code page, and a UTF-8 cluster then rejects any non-ASCII byte
// ("invalid byte sequence for encoding UTF8"). A user profile such as
// C:\Users\Müller is enough to trigger it. macOS and Linux paths are UTF-8, so
// they are used as given.
func ResolveRoot(preferred string) (string, error) {
	if preferred == "" {
		return "", fmt.Errorf("desktop: empty data directory")
	}
	abs, err := filepath.Abs(preferred)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return "", fmt.Errorf("desktop: create %s: %w", abs, err)
	}
	if isASCII(abs) {
		return abs, nil
	}
	return asciiFallback(abs)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
