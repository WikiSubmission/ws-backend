// Package textnorm is the Go side of the offline search-text normalization
// contract. It MUST produce byte-identical output to the TypeScript
// normalizeForSearch in org (packages/shared/lib/text-normalization/normalize.ts)
// for every case in the shared vectors, otherwise offline search — whose index
// is built here and queried on the client — silently returns wrong results.
//
// The behavior is frozen by testdata/normalization_vectors.json, which is a
// verbatim copy of the org vectors file (synced like openapi.yaml). See
// org/docs/text-normalization.md for the full specification.
package textnorm

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Version mirrors NORMALIZATION_VERSION in the TypeScript implementation and is
// recorded in each bundle's meta table. Bump in lockstep with the TS constant
// and the shared vectors.
const Version = 1

const tatweel = 'ـ'

// fold maps the residual Arabic letterforms that have no canonical NFD
// decomposition (plus the hamza carriers, kept for idempotency on non-NFD
// input) to their search-folded base letter. Mirrors FOLD in the TS impl.
var fold = map[rune]string{
	'ٱ': "ا", // ٱ alef wasla       -> ا
	'أ': "ا", // أ alef hamza above -> ا
	'إ': "ا", // إ alef hamza below -> ا
	'آ': "ا", // آ alef madda       -> ا
	'ى': "ي", // ى alef maqsura     -> ي
	'ة': "ه", // ة ta marbuta       -> ه
	'ؤ': "و", // ؤ waw hamza        -> و
	'ئ': "ي", // ئ ya hamza         -> ي
	'ء': "",       // ء standalone hamza -> removed
}

// ForSearch normalizes a string for indexing or querying the offline FTS index.
//
// Pipeline (identical to the TS impl): NFD decompose -> drop nonspacing marks
// (unicode.Mn) -> drop tatweel -> fold alef/waw/ya/ta-marbuta variants ->
// lowercase -> collapse whitespace. Word stemming is left to the FTS5 tokenizer.
func ForSearch(s string) string {
	if s == "" {
		return ""
	}

	decomposed := norm.NFD.String(s)
	var b strings.Builder
	b.Grow(len(decomposed))
	for _, r := range decomposed {
		if r == tatweel {
			continue
		}
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if rep, ok := fold[r]; ok {
			b.WriteString(rep)
			continue
		}
		b.WriteRune(r)
	}

	// strings.Fields splits on unicode whitespace and drops empties; rejoining
	// with a single space both collapses internal runs and trims the ends.
	return strings.Join(strings.Fields(strings.ToLower(b.String())), " ")
}
