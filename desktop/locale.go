package desktop

import "runtime"

// clusterLocale is the initdb locale for the desktop cluster.
//
// It must be a UTF-8 locale whose character classes know non-ASCII text. The
// full-text search parser takes word boundaries from LC_CTYPE: under the C
// locale it treats "—", "«", "’" and the Arabic comma "،" as letters, so
// "patience—and" becomes one token and search results and highlights differ
// from the hosted server (en_US.utf8 on Linux). The builtin C.UTF-8 provider
// does not help, because the parser still uses the libc locale.
//
// The name of such a locale differs per OS, and it has to exist on every
// user's machine, hence one known-present choice each.
func clusterLocale() string {
	switch runtime.GOOS {
	case "windows":
		// BCP 47 names are always available on Windows.
		return "en-US"
	case "darwin":
		return "en_US.UTF-8"
	default:
		// Built into glibc 2.35+ and musl; en_US.UTF-8 is often not generated
		// on minimal installs.
		return "C.UTF-8"
	}
}
