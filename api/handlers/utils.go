package handlers

import "fmt"

func (s *Server) ValidateQuranRange(sCh, eCh, sV, eV int) (int, int, int, int, error) {
	// Check Chapter bounds
	startMeta, sExists := s.Chapters[sCh]
	endMeta, eExists := s.Chapters[eCh]
	outsCh, outeCh, outsV, outeV := 0, 0, 0, 0
	err := ""

	if !sExists || !eExists {
		return outsCh, outeCh, outsV, outeV, fmt.Errorf("invalid chapter range: %d-%d", sCh, eCh)
	}

	// Validate Verses
	if sV < 0 {
		outsV = 0
		err = fmt.Sprintf("%s, chapter %d only has %d verses", err, sCh, startMeta.VerseCount)
	}
	if sV > startMeta.VerseCount {
		outsV = startMeta.VerseCount
		err = fmt.Sprintf("%s, chapter %d only has %d verses", err, sCh, startMeta.VerseCount)
	}

	if eV != 9999 {
		if eV < 0 {
			outeV = 0
			err = fmt.Sprintf("%s, chapter %d only has %d verses", err, sCh, endMeta.VerseCount)
		}
		if eV > endMeta.VerseCount {
			outeV = startMeta.VerseCount
			err = fmt.Sprintf("%s, chapter %d only has %d verses", err, sCh, endMeta.VerseCount)
		}
	}

	if sCh == eCh && eV < sV {
		err = fmt.Sprintf("%s, end verse cannot be less than start verse", err)
	}

	return outsCh, outeCh, outsV, outeV, fmt.Errorf("%s", err)
}
