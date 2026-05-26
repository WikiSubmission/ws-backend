package handlers

// Stubs for Quran Games — Fill the Missing Word.
//
// See docs/games-spec.md for the v1 design. The read endpoints (passages,
// leaderboard, history, stats) return empty payloads until the curation and
// scoring pipelines land; the write endpoints (start variant, submit attempt)
// return 501 NotImplemented.

import (
	"context"
)

// GET /games/fill-blank/passages
func (s *Server) ListGameFillBlankPassages(_ context.Context, _ ListGameFillBlankPassagesRequestObject) (ListGameFillBlankPassagesResponseObject, error) {
	return ListGameFillBlankPassages200JSONResponse{
		Data: []GamePassage{},
	}, nil
}

// POST /games/fill-blank/variants
func (s *Server) StartGameFillBlankVariant(_ context.Context, _ StartGameFillBlankVariantRequestObject) (StartGameFillBlankVariantResponseObject, error) {
	return StartGameFillBlankVariant501JSONResponse{
		NotImplementedErrorJSONResponse: NotImplementedErrorJSONResponse{
			Message: "Fill-the-Blank variant generation is not yet implemented.",
			Code:    "NOT_IMPLEMENTED",
		},
	}, nil
}

// POST /games/fill-blank/submit
func (s *Server) SubmitGameFillBlankAttempt(_ context.Context, _ SubmitGameFillBlankAttemptRequestObject) (SubmitGameFillBlankAttemptResponseObject, error) {
	return SubmitGameFillBlankAttempt501JSONResponse{
		NotImplementedErrorJSONResponse: NotImplementedErrorJSONResponse{
			Message: "Fill-the-Blank submission is not yet implemented.",
			Code:    "NOT_IMPLEMENTED",
		},
	}, nil
}

// GET /games/fill-blank/leaderboard
func (s *Server) GetGameFillBlankLeaderboard(_ context.Context, _ GetGameFillBlankLeaderboardRequestObject) (GetGameFillBlankLeaderboardResponseObject, error) {
	return GetGameFillBlankLeaderboard200JSONResponse{
		Data: []GameLeaderboardEntry{},
	}, nil
}

// GET /me/games/history
func (s *Server) ListMeGameHistory(_ context.Context, _ ListMeGameHistoryRequestObject) (ListMeGameHistoryResponseObject, error) {
	return ListMeGameHistory200JSONResponse{
		Data: []GameHistoryEntry{},
	}, nil
}

// GET /me/games/stats
func (s *Server) GetMeGameStats(_ context.Context, _ GetMeGameStatsRequestObject) (GetMeGameStatsResponseObject, error) {
	return GetMeGameStats200JSONResponse{
		Data: GameStats{
			TotalAttempts:     0,
			BestScore:         0,
			VariantsCompleted: 0,
			ByDifficulty: struct {
				Easy         *GameStatsByDifficulty `json:"easy,omitempty"`
				Hard         *GameStatsByDifficulty `json:"hard,omitempty"`
				Medium       *GameStatsByDifficulty `json:"medium,omitempty"`
				Professional *GameStatsByDifficulty `json:"professional,omitempty"`
			}{},
		},
	}, nil
}
