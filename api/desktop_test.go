package main

import "testing"

func TestDesktopRouteAllowed(t *testing.T) {
	allowed := []string{
		"/health",
		"/api/v1/quran",
		"/api/v1/search",
		"/api/v1/roots/ktb",
		"/api/v1/roots/ktb/occurrences",
		"/api/v1/topic-index/verse/2:255",
		"/api/v1/editorial/public/appendices",
		"/api/v1/editorial/public/appendices/en/1",
		"/api/v1/users/me",
		"/api/v1/me/notes",
		"/api/v1/me/notes/12",
		"/api/v1/me/collections/3/verses/7",
	}
	blocked := []string{
		"/metrics",
		"/api/v1/bible",
		"/api/v1/bible/books",
		"/api/v1/music/tracks",
		"/api/v1/library/search",
		"/api/v1/site/search",
		"/api/v1/editorial/public/articles",
		"/api/v1/editorial/public/appendicesX",
		"/api/v1/editorial/session",
		"/api/v1/admin/offline/files",
		"/api/v1/auth/mobile/exchange",
		"/api/v1/games/fill-blank/passages",
		"/api/v1/me/export",
		"/api/v1/me/sync",
		"/api/v1/me/push/subscribe",
		"/api/v1/me/delete-account",
		"/api/v1/me/notesX",
		"/api/v1/me/collection-subscriptions",
		"/api/v1/collections/share/abc",
		"/api/v1/users",
		"/api/v1/users/5",
		"/api/v1/quranX",
	}
	for _, p := range allowed {
		if !desktopRouteAllowed(p) {
			t.Errorf("expected %s to be allowed", p)
		}
	}
	for _, p := range blocked {
		if desktopRouteAllowed(p) {
			t.Errorf("expected %s to be blocked", p)
		}
	}
}
