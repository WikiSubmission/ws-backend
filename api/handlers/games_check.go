package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/db"
)

// GamesPlayHandler serves lightweight, in-play gameplay endpoints that sit
// outside the generated OpenAPI surface (registered before the validator,
// like the editor routes).
type GamesPlayHandler struct {
	DB *db.DB
}

type checkBlankRequest struct {
	VariantID string `json:"variant_id"`
	SessionID string `json:"session_id"`
	Index     int    `json:"index"`
	Guess     string `json:"guess"`
}

// CheckBlank validates a single blank guess and returns only whether it is
// correct. It deliberately never returns the accepted answer, so the play
// screen can give instant green/red feedback without leaking answers (which
// would make the leaderboard cheatable). Final scoring still happens on
// submit, which remains the authoritative, rate-limited path.
func (h GamesPlayHandler) CheckBlank(c *gin.Context) {
	var req checkBlankRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}
	if req.VariantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "variant_id is required"})
		return
	}

	ctx := c.Request.Context()
	variant, err := h.DB.FetchVariantByVariantID(ctx, req.VariantID)
	if err != nil {
		slog.Error("CheckBlank: FetchVariantByVariantID", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	if variant == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "variant not found"})
		return
	}

	var blanks []blankRecord
	if err := json.Unmarshal(variant.Blanks, &blanks); err != nil {
		slog.Error("CheckBlank: unmarshal blanks", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}

	var target *blankRecord
	for i := range blanks {
		if blanks[i].Index == req.Index {
			target = &blanks[i]
			break
		}
	}
	if target == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "blank not found"})
		return
	}

	// Hard tier accepts lemma matches, so load lemmas for just this guess and
	// answer (mirrors the submit path).
	var lemmaMap map[string]string
	if variant.Difficulty == "hard" {
		words := []string{normalizeAnswer(target.Answer), normalizeAnswer(req.Guess)}
		if m, lerr := h.DB.FetchLemmasFor(ctx, variant.Language, words); lerr == nil {
			lemmaMap = m
		}
	}

	correct := scoreGuess(req.Guess, target, variant.Difficulty, lemmaMap)

	maxAttempts := maxAttemptsForDifficulty(variant.Difficulty)
	if maxAttempts == 0 || req.SessionID == "" {
		// Easy difficulty or no session: no attempt tracking.
		c.JSON(http.StatusOK, gin.H{"correct": correct})
		return
	}

	attemptsRemaining := maxAttempts
	if !correct {
		wrongCount, err := h.DB.IncrementBlankWrong(ctx, req.SessionID, req.Index)
		if err != nil {
			slog.Error("CheckBlank: IncrementBlankWrong", "error", err)
			// Non-fatal: still return the correctness result.
			c.JSON(http.StatusOK, gin.H{"correct": correct, "attempts_remaining": maxAttempts - 1})
			return
		}
		attemptsRemaining = max(0, maxAttempts-wrongCount)
	}

	c.JSON(http.StatusOK, gin.H{
		"correct":            correct,
		"attempts_remaining": attemptsRemaining,
	})
}
