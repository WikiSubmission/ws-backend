package handlers

import "fmt"

func (s *Server) ValidateQuranRange(sCh, eCh, sV, eV int) error {
	// Check Chapter bounds
	startMeta, sExists := s.Chapters[sCh]
	endMeta, eExists := s.Chapters[eCh]

	if !sExists || !eExists {
		return fmt.Errorf("invalid chapter range: %d-%d", sCh, eCh)
	}

	// Validate Verses
	if sV < 1 || sV > startMeta.VerseCount {
		return fmt.Errorf("chapter %d only has %d verses", sCh, startMeta.VerseCount)
	}

	if eV != 9999 && (eV < 1 || eV > endMeta.VerseCount) {
		return fmt.Errorf("chapter %d only has %d verses", eCh, endMeta.VerseCount)
	}

	if sCh == eCh && eV < sV {
		return fmt.Errorf("end verse cannot be less than start verse")
	}

	return nil
}
