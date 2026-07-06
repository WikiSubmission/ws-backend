// Command build-offline-bundles renders the offline content bundles (one
// SQLite file per scripture+language) plus the manifest, reading from the same
// Postgres the API uses. It targets the Quran (ar, en text bundles by default)
// and additionally builds word-by-word bundles for the languages in
// -word-langs (en by default; each carries ar + target + tl word rows). It
// writes to a local output directory; with -upload it then publishes the
// bundles and manifest to the public CDN through ws-lib's store-public
// endpoint (see offline/publish), so they become fetchable at -base-url.
//
// The same build+publish pipeline is exposed to admins as POST
// /admin/offline/rebuild (see api/handlers/offline_admin.go); both paths share
// offline/builder.
//
// Usage:
//
//	go run ./cmd/build-offline-bundles -out offline-bundles -langs ar,en -word-langs en -data-version 1
//	go run ./cmd/build-offline-bundles -langs ar,en -data-version 1 -upload
//
// Environment: same DATABASE_* / DATABASE_URL vars as the API (loaded from .env
// when present), since it connects via db.ConnectDB. -upload additionally
// requires WSLIB_BASE_URL and STORE_AUTH_SECRET (same pair as the data-export
// uploader in api/server.go).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/wikiSubmission/ws-backend/db"
	"github.com/wikiSubmission/ws-backend/offline/builder"
	"github.com/wikiSubmission/ws-backend/offline/publish"
)

func main() {
	out := flag.String("out", "offline-bundles", "output directory for bundle files and manifest")
	langsCSV := flag.String("langs", "ar,en", "comma-separated Quran language codes to build")
	wordLangsCSV := flag.String("word-langs", "en", "comma-separated languages to build word-by-word bundles for (empty to skip)")
	libraryLangsCSV := flag.String("library-langs", "en", "comma-separated languages to build library-document bundles for (empty to skip)")
	baseURL := flag.String("base-url", "https://cdn.wikisubmission.org/offline", "base URL the manifest points bundle downloads at")
	dataVersion := flag.Int("data-version", 1, "data_version stamped into every bundle and its versioned filename")
	upload := flag.Bool("upload", false, "publish bundles and manifest to the CDN via ws-lib (requires WSLIB_BASE_URL and STORE_AUTH_SECRET)")
	flag.Parse()

	if err := godotenv.Load(); err != nil {
		fmt.Println("info: no .env file found, using system environment variables")
	}

	var store *publish.WSLibPublicStore
	if *upload {
		wslibURL := strings.TrimRight(os.Getenv("WSLIB_BASE_URL"), "/")
		secret := os.Getenv("STORE_AUTH_SECRET")
		if wslibURL == "" || secret == "" {
			log.Fatal("build-offline-bundles: -upload requires WSLIB_BASE_URL and STORE_AUTH_SECRET to be set")
		}
		store = &publish.WSLibPublicStore{
			BaseURL: wslibURL,
			Secret:  []byte(secret),
			Client:  &http.Client{Timeout: 2 * time.Minute},
		}
	}

	database, err := db.ConnectDB()
	if err != nil {
		log.Fatalf("build-offline-bundles: connect db: %v", err)
	}
	defer database.Close()

	_, err = builder.Run(context.Background(), database, builder.Options{
		Langs:        splitCSV(*langsCSV),
		WordLangs:    splitCSV(*wordLangsCSV),
		LibraryLangs: splitCSV(*libraryLangsCSV),
		OutDir:       *out,
		BaseURL:      strings.TrimRight(*baseURL, "/"),
		DataVersion:  *dataVersion,
		Store:        store,
	})
	if err != nil {
		log.Fatalf("build-offline-bundles: %v", err)
	}
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
