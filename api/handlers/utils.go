package handlers

import "fmt"

func (s *Server) ValidateQuranRange(sCh, eCh, sV, eV int) (int, int, int, int, error) {
	// Fix inverted chapter ranges FIRST so verse clamping uses the correct metadata.
	if eCh < sCh {
		sCh, eCh = eCh, sCh
	}

	// Check Chapter bounds
	startMeta, sExists := s.Chapters[sCh]
	endMeta, eExists := s.Chapters[eCh]
	if !sExists || !eExists {
		return 0, 0, 0, 0, fmt.Errorf("invalid chapter range: %d-%d", sCh, eCh)
	}

	// Default full chapter behavior (0 is valid — it's the Basmallah verse index)
	if sV < 0 {
		sV = 0
	}

	if eV == 9999 {
		eV = endMeta.VerseCount
	}

	// Clamp verses to valid range
	if sV > startMeta.VerseCount {
		sV = startMeta.VerseCount
	}

	if eV > endMeta.VerseCount {
		eV = endMeta.VerseCount
	}

	if eV < 1 {
		eV = 1
	}

	// Fix inverted verse ranges (only meaningful within the same chapter)
	if sCh == eCh && eV < sV {
		sV, eV = eV, sV
	}

	return sCh, eCh, sV, eV, nil
}
