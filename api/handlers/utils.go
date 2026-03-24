package handlers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// VerseSpec is one parsed token from the `verses` query parameter.
// End == -1 means a single verse; otherwise it is an inclusive range.
type VerseSpec struct {
	Chapter int
	Start   int
	End     int
}

// VerseRef is a single resolved (chapter, verse_index) pair ready for DB use.
type VerseRef struct {
	Chapter    int32
	VerseIndex int32
}

var versePartRe = regexp.MustCompile(`^(-?\d+)(?:-(\d+))?$`)

// ParseVerseRefs parses a raw `verses` query string into a slice of VerseSpec.
// Accepted formats (comma-separated): "1:1", "1:1-3", "1:2,2:255,36:1-5".
// Does NOT validate chapter/verse existence — that is left to ExpandVerseRefs.
func ParseVerseRefs(raw string) ([]VerseSpec, error) {
	var specs []VerseSpec
	for _, token := range strings.Split(raw, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		parts := strings.SplitN(token, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid verse reference %q: expected format chapter:verse or chapter:start-end", token)
		}
		ch, err := strconv.Atoi(parts[0])
		if err != nil || ch < 1 {
			return nil, fmt.Errorf("invalid chapter number in %q", token)
		}
		m := versePartRe.FindStringSubmatch(parts[1])
		if m == nil {
			return nil, fmt.Errorf("invalid verse spec in %q: expected integer or integer-integer", token)
		}
		start, _ := strconv.Atoi(m[1])
		end := -1
		if m[2] != "" {
			end, err = strconv.Atoi(m[2])
			if err != nil {
				return nil, fmt.Errorf("invalid verse end in %q", token)
			}
		}
		specs = append(specs, VerseSpec{Chapter: ch, Start: start, End: end})
	}
	if len(specs) == 0 {
		return nil, fmt.Errorf("verses parameter is empty")
	}
	return specs, nil
}

// ExpandVerseRefs validates each VerseSpec against the chapter cache, clamps
// verse indices, expands ranges into individual VerseRef pairs, deduplicates,
// and enforces the maxVerses limit. Negative verse indices are clamped to 0
// (the Basmallah verse), consistent with ValidateQuranRange behaviour.
func (s *Server) ExpandVerseRefs(specs []VerseSpec, maxVerses int) ([]VerseRef, error) {
	seen := make(map[string]struct{})
	var refs []VerseRef

	for _, spec := range specs {
		meta, ok := s.Chapters[spec.Chapter]
		if !ok {
			return nil, fmt.Errorf("unknown chapter: %d", spec.Chapter)
		}

		// Clamp start verse
		start := spec.Start
		if start < 0 {
			start = 0
		}
		if start > meta.VerseCount {
			start = meta.VerseCount
		}

		if spec.End == -1 {
			// Single verse
			key := fmt.Sprintf("%d:%d", spec.Chapter, start)
			if _, dup := seen[key]; !dup {
				seen[key] = struct{}{}
				refs = append(refs, VerseRef{Chapter: int32(spec.Chapter), VerseIndex: int32(start)})
				if len(refs) > maxVerses {
					return nil, fmt.Errorf("too many verses requested (max %d)", maxVerses)
				}
			}
		} else {
			// Range
			end := spec.End
			if end < 1 {
				end = 1
			}
			if end > meta.VerseCount {
				end = meta.VerseCount
			}
			if end < start {
				start, end = end, start
			}
			for vi := start; vi <= end; vi++ {
				key := fmt.Sprintf("%d:%d", spec.Chapter, vi)
				if _, dup := seen[key]; !dup {
					seen[key] = struct{}{}
					refs = append(refs, VerseRef{Chapter: int32(spec.Chapter), VerseIndex: int32(vi)})
					if len(refs) > maxVerses {
						return nil, fmt.Errorf("too many verses requested (max %d)", maxVerses)
					}
				}
			}
		}
	}
	return refs, nil
}

// refsToArrays splits a []VerseRef into two parallel int32 slices for use
// as PostgreSQL array parameters in unnest-based DB queries.
func refsToArrays(refs []VerseRef) (chNums, viNums []int32) {
	chNums = make([]int32, len(refs))
	viNums = make([]int32, len(refs))
	for i, r := range refs {
		chNums[i] = r.Chapter
		viNums[i] = r.VerseIndex
	}
	return
}

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
