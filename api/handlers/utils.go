package handlers

import "fmt"

func (s *Server) ValidateQuranRange(sCh, eCh, sV, eV int) (int, int, int, int, error) {
	// Check Chapter bounds
	startMeta, sExists := s.Chapters[sCh]
	endMeta, eExists := s.Chapters[eCh]
	if !sExists || !eExists {
		return 0, 0, 0, 0, fmt.Errorf("invalid chapter range: %d-%d", sCh, eCh)
	}

	// Default full chapter behavior
	if sV <= 0 {
		sV = 1
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

	// Fix inverted verse ranges
	if sCh == eCh && eV < sV {
		sV, eV = eV, sV
	}

	// Fix inverted chapter ranges
	if eCh < sCh {
		sCh, eCh = eCh, sCh
	}

	return sCh, eCh, sV, eV, nil
}
