package record

// STORE-SIDE FILE, NOT IMPORTER: this read is SQL over `links`, `records` and
// `heads` - the store's own schema - and section 50's move 2 (B100) is what
// moved it out of brief.go, which now holds derivation only.

import (
	"context"
	"encoding/json"
	"fmt"
)

// Note is section 39 row 3's note, RENDERED IN FULL.
//
// "never summarised - this is Boris's comment/question mechanism, and an agent
// that skips it has not read the item." So the body is carried whole. That is
// the opposite of the compact card's rule and both are deliberate: the card is
// a list to scan, this is a question somebody asked and is waiting on.
type Note struct {
	ID string

	// Title is what the document called this note, and it is what a reader
	// needs FIRST.
	//
	// ⛔ IT WAS DERIVED AND THEN DROPPED, WHICH IS WHY THE WINDOW SHOWED PROSE
	// WHERE A NAME BELONGS. Every one of the eight notes in Boris's store
	// carries `fields["title"]` - "Naming", "What rig is" - and this struct had
	// no slot for it, so the window listed each note by the first 90 characters
	// of its BODY. One of his is 1,951 bytes and one is empty, so that list
	// read as a truncated paragraph and a blank line. Boris, 2026-09-18: "fix
	// the rig functionality". B97.
	Title string

	Body     string
	Priority string

	// About is the record this note is part-of - the project itself, or one of
	// its work-items. A note with no About has nothing to be read against.
	About string

	Prov Provenance
}

// NotesAbout collects section 3's notes: every note part-of one of `subjects`.
//
// ONE QUERY FOR EVERY NOTE IN THE PROJECT, FILTERED IN GO. The alternative is
// an IN clause built from the subject set, which is a query whose text changes
// with the data - unprepareable, and a different plan on every call. The set is
// the project plus its open items, so the difference is a handful of rows.
//
// ⛔ SCOPED ON THE DESTINATION, like the has-note flag, because section 39 rules
// that links MAY cross a project boundary. A note written elsewhere and attached
// here is exactly the question this section exists to surface.
//
// ⛔ THE ROWS COME BACK IN ORDER BY n.id AND THE RANK IS APPLIED BY THE
// CALLER, which is B100's seam: importance is the brief's policy and the
// store does not know which note matters. brief.go's sortNotes is the one
// place that decides, and both note lists go through it. The paragraphs below
// are why that rank exists and they belong with it.
//
// ⛔ ORDERED BY PRIORITY, THEN created_at DESCENDING, THEN id - AND THIS
// COMMENT USED TO NAME A MECHANISM THE QUERY DID NOT HAVE. It read "ordered by
// priority then id" above an ORDER BY n.id, wrong from the first draft rather
// than drifted, and no test ever watched it: ordering by id alone IS
// deterministic, so every assertion here passed against a caption describing a
// sort that was not happening.
//
// Section 39 binds neither order here - row 3 says a note is rendered in full
// and says nothing about sequence - so this one is the seat's, taken with the
// lead. It is deliberately the SAME sort a case's attention_n notes get eleven
// rows later in section 39, because two adjacent note lists ordering
// differently is precisely the drift the one-definition rule exists to stop.
//
// ⛔ AND THE SORT RUNS HERE RATHER THAN IN THE ORDER BY, WHICH IS THE ONE THING
// THE RANK CANNOT DELEGATE. SQLite can only reach the raw string out of the
// fields blob, so a SQL ordering is alphabetical whichever way it is pointed.
// The only SQL form that honours the rank is a CASE WHEN, and that puts a
// second copy of the vocabulary in a string literal no Go test can reach -
// which is the drift the ruling names, arriving through the door opened to
// implement it. The query keeps ORDER BY n.id for a stable read.
func (s *Store) NotesAbout(ctx context.Context, project string, subjects map[string]bool) ([]Note, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT n.id, n.body, n.fields, l.dst,
		       n.session, n.seat, n.epoch, n.created_at
		FROM links l
		JOIN records n ON n.id = l.src
		JOIN heads hn ON hn.id = n.id AND hn.version = n.version
		JOIN records d ON d.id = l.dst
		JOIN heads hd ON hd.id = d.id AND hd.version = d.version
		WHERE l.type = ? AND n.kind = ? AND d.project = ?
		ORDER BY n.id`, LinkPartOf, KindNote, project)
	if err != nil {
		return nil, fmt.Errorf("record: reading the notes in %s: %w", project, err)
	}
	defer func() { _ = rows.Close() }()

	var out []Note
	for rows.Next() {
		var (
			n           Note
			fields      string
			epoch, nano int64
		)
		if err := rows.Scan(&n.ID, &n.Body, &fields, &n.About,
			&n.Prov.Session, &n.Prov.Seat, &epoch, &nano); err != nil {
			return nil, err
		}
		if !subjects[n.About] {
			continue
		}
		if n.Prov.Epoch, err = fromColumn("epoch", epoch); err != nil {
			return nil, err
		}
		n.Prov.CreatedAt = unixNano(nano)
		var f map[string]string
		if err := json.Unmarshal([]byte(fields), &f); err == nil {
			n.Priority = f["priority"]
			n.Title = f["title"]
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}
