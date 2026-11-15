//go:build !windows

package desktop

// asciiFallback is never needed off Windows: Postgres handles UTF-8 paths.
func asciiFallback(abs string) (string, error) {
	return abs, nil
}
