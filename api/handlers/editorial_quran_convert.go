package handlers

import (
	"encoding/json"

	"github.com/wikiSubmission/ws-backend/db"
)

// Wire converters: db row structs -> generated OpenAPI models. Kept separate
// from the handlers so the mapping is easy to scan and unit-test.

func toQuranTextFields(t *db.QuranText) *QuranTextFields {
	if t == nil {
		return nil
	}
	return &QuranTextFields{Subtitle: t.Subtitle, Content: t.Content, Footer: t.Footer}
}

func toQuranVerseDraft(r db.QuranVerseEditorRow) QuranVerseDraft {
	d := QuranVerseDraft{
		VerseNumber:    r.VerseNumber,
		Draft:          toQuranTextFields(r.Draft),
		DraftUpdatedAt: r.DraftUpdatedAt,
		Published:      toQuranTextFields(r.Published),
		Reference:      toQuranTextFields(r.Reference),
	}
	if r.DraftStatus != nil {
		s := QuranPublishStatus(*r.DraftStatus)
		d.DraftStatus = &s
	}
	return d
}

func toPublishRequest(r *db.PublishRequestRow) QuranPublishRequest {
	out := QuranPublishRequest{
		Id:            int64(r.ID),
		VersionId:     int64(r.VersionID),
		ChapterNumber: r.ChapterNumber,
		Status:        QuranPublishStatus(r.Status),
		Note:          r.Note,
		RequestedBy:   int64(r.RequestedBy),
		RequestedAt:   r.RequestedAt,
		ReviewedAt:    r.ReviewedAt,
		ReviewNote:    r.ReviewNote,
	}
	if r.ReviewedBy != nil {
		v := int64(*r.ReviewedBy)
		out.ReviewedBy = &v
	}
	return out
}

func toChangelogEntry(r db.ChangelogRow) QuranChangelogEntry {
	e := QuranChangelogEntry{
		Id:            int64(r.ID),
		VersionId:     int64(r.VersionID),
		ChapterNumber: r.ChapterNumber,
		VerseNumber:   r.VerseNumber,
		Action:        r.Action,
		ActorId:       int64(r.ActorID),
		CreatedAt:     r.CreatedAt,
	}
	if r.PublishRequestID != nil {
		v := int64(*r.PublishRequestID)
		e.PublishRequestId = &v
	}
	if len(r.Detail) > 0 && string(r.Detail) != "{}" {
		var m map[string]any
		if err := json.Unmarshal(r.Detail, &m); err == nil && len(m) > 0 {
			e.Detail = &m
		}
	}
	return e
}
