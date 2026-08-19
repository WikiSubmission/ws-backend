package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/wikiSubmission/ws-backend/db"
)

// Embedder generates query embeddings for semantic search.
// Defined here (where it is consumed) per the Go "accept interfaces" idiom.
// The concrete *embed.OpenAIEmbedder satisfies this interface.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

type LanguageCache struct {
	ID        int
	Code      string
	Direction string
}

type ChapterCache struct {
	VerseCount      int
	RevelationOrder int
}

type BibleBookCache struct {
	Name         string
	Testament    string
	ChapterCount int
}

type Server struct {
	DB              *db.DB
	Languages       map[string]LanguageCache
	Chapters        map[int]ChapterCache
	BibleBooks      map[int]BibleBookCache
	QuranLangs      map[string]bool // lang codes with Quran verse_text content
	BibleLangs      map[string]bool // lang codes with Bible verse_text content
	SiteLangs       map[string]bool // UI locales with indexed site_documents content
	TopicIndexLangs map[string]bool // index editions with topic_index_entries content
	Embedder        Embedder        // nil = FTS-only mode (OPENAI_API_KEY not set)
	ExportUploader  ExportUploader  // nil = user data export upload disabled
	PushSender      WebPushSender   // zero value = push sending disabled

	// Games rate limiters. Lazily allocated by the games handlers if not set
	// by the caller, so tests do not need to wire them explicitly.
	gamesLimits     *gamesLimiters
	gamesLimitsOnce sync.Once
}

func (s *Server) gamesLimitsOrInit() *gamesLimiters {
	s.gamesLimitsOnce.Do(func() {
		if s.gamesLimits == nil {
			s.gamesLimits = newGamesLimiters()
		}
	})
	return s.gamesLimits
}

func (s *Server) InitializeCaches(ctx context.Context) error {
	// 1. Load Languages
	langs, err := s.DB.FetchAllLanguages(ctx)
	if err != nil {
		return fmt.Errorf("lang cache: %w", err)
	}

	s.Languages = make(map[string]LanguageCache)
	for _, l := range langs {
		s.Languages[l.Code] = LanguageCache{
			ID: l.ID, Code: l.Code, Direction: l.Direction,
		}
	}

	// 2. Load Chapter Metadata
	meta, err := s.DB.FetchAllChapterMeta(ctx)
	if err != nil {
		return fmt.Errorf("chapter cache: %w", err)
	}

	s.Chapters = make(map[int]ChapterCache)
	for _, m := range meta {
		s.Chapters[m.ChapterNumber] = ChapterCache{
			VerseCount:      m.VerseCount,
			RevelationOrder: m.RevelationOrder,
		}
	}

	// 3. Load Bible book metadata
	bibleBooks, err := s.DB.FetchBibleBooks(ctx)
	if err != nil {
		return fmt.Errorf("bible book cache: %w", err)
	}
	s.BibleBooks = make(map[int]BibleBookCache)
	for _, b := range bibleBooks {
		s.BibleBooks[b.BookNumber] = BibleBookCache{
			Name:         b.Name,
			Testament:    b.Testament,
			ChapterCount: b.ChapterCount,
		}
	}

	// 4. Load Quran language set (for FTS language detection)
	quranLangs, err := s.DB.FetchQuranLangs(ctx)
	if err != nil {
		return fmt.Errorf("quran langs cache: %w", err)
	}
	s.QuranLangs = quranLangs

	// 5. Load Bible language set (for FTS language detection)
	bibleLangs, err := s.DB.FetchBibleLangs(ctx)
	if err != nil {
		return fmt.Errorf("bible langs cache: %w", err)
	}
	s.BibleLangs = bibleLangs

	// 6. Load the UI locales with indexed site content. Not fatal when the
	// catalogue has not been seeded yet: /site/search then simply finds nothing,
	// rather than the whole server refusing to start.
	siteLangs, err := s.DB.FetchSiteLangs(ctx)
	if err != nil {
		slog.Warn("site langs cache unavailable; /site/search will accept any lang", "err", err)
		s.SiteLangs = map[string]bool{}
	} else {
		s.SiteLangs = siteLangs
	}

	// 7. Load the topical index editions. Same non-fatal handling as the site
	// catalogue: an unseeded index means /topic-index finds nothing, not a server
	// that refuses to start.
	topicLangs, err := s.DB.FetchTopicIndexLangs(ctx)
	if err != nil {
		slog.Warn("topic index langs cache unavailable; /topic-index will accept any lang", "err", err)
		s.TopicIndexLangs = map[string]bool{}
	} else {
		s.TopicIndexLangs = topicLangs
	}

	return nil
}

// // 8. Fetch a range of verses
// func (s *Server) GetQuran(ctx context.Context, request GetQuranRequestObject) (GetQuranResponseObject, error) {
// 	return GetQuran501JSONResponse{
// 		NotImplementedErrorJSONResponse{
// 			Message: "GetQuran is not implemented yet",
// 			Code:    "NOT_IMPLEMENTED",
// 		},
// 	}, nil
// }
