// Package worknote is an agent's working notes: prose it writes as it works,
// attached to anything in the record, tagged however it likes, and handed back
// to the SEAT that wrote it however many processes have died in between.
//
// ⛔ BORIS, 2026-09-17, VERBATIM, and plan/09 records the turn he said it:
// "The AI agent working with rig should manage its scratch-pads in rig so that
// nothing is ever lost. It can associate its scratch-pads to anything in rig
// it wants, for example a work-item, a project, a task, anything it wants, it
// can also set any tags it wants ... I want to make AI agents
// first-class-citizens."
//
// ⛔ WHY THIS PACKAGE EXISTS RATHER THAN THREE MORE METHODS ON internal/record.
// plan/50 decision 5 keeps the STORE kind-agnostic: it does not know what its
// callers' kinds mean, and every kind that behaved specially moved out of it.
// A working note is a kind with a vocabulary - a name, a tag encoding, a
// retrieval question - so the vocabulary lives here and the store below stays
// a store. `record.Append`, `record.FindBySeat` and `record.FindAttachedTo`
// name no kind at all, which is the line this package is on the far side of.
package worknote

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/borismilner/rig/internal/record"
)

// Kind is what a working note IS, in the store.
//
// ⛔ `working-note` AND NOT `scratchpad`, AND NOT SECTION 39's `note`. plan/09
// A5 rules both halves. A scratchpad is by definition the thing you throw
// away and his acceptance test is that nothing is ever lost, so the word
// fights the requirement. And section 39's `note` is a PROJECT note with
// `priority` and `about`, rendered in a brief a person reads: an agent's
// working notes are high-volume and would flood that surface. Same word,
// different lifecycle.
//
// ⛔ THE HYPHEN IS LOAD-BEARING AND THE SURFACE WORD IS NOT THE KIND. The
// verbs, the tools and the CLI all say `worknote`; the stored kind says
// `working-note`. TestTheStoredKindAndTheSurfaceWordCannotDrift pins the two
// together, because a kind is matched EXACTLY by record.query and a rename on
// one side would leave every note ever written unfindable by the other.
const Kind = "working-note"

// TagPrefix is how a tag is stored: one field per tag, `tag:<name>` = `1`.
//
// ⛔ ONE FIELD PER TAG RATHER THAN A COMMA-JOINED `tags` FIELD, AND THE REASON
// IS B65 RATHER THAN TASTE. plan/09 A3 gives an agent any tags it wants, and
// the paragraph under it says tags that cannot be QUERIED "are worse than no
// tags, because they look like an index". The store's field predicate
// (QueryFilter.Field/Value) matches a key and an EXACT value, so one field per
// tag is selectable today through the record.query that already ships:
// `--field tag:review --value 1`. A joined `tags: "review,api"` would have
// needed a LIKE, which the predicate does not do and which would match
// `reviewed` as well.
//
// The value is `1` and not the tag name again, so the value side of the
// predicate is a constant a caller cannot get wrong.
const (
	TagPrefix = "tag:"
	TagValue  = "1"
)

// MaxTags bounds the tags on one note.
//
// ⛔ A BOUND ON WORK DRIVEN BY A REMOTE CALLER, like record.MaxAttachments.
// Each tag is a key in the note's fields map, the map is serialised as JSON
// into a column, and an unbounded list is an unbounded row. Sixteen is the
// same number section 40's lessons take, chosen there for the same reason.
const MaxTags = 16

// MaxTagLen bounds one tag.
const MaxTagLen = 64

// MaxBody is the largest note body, matching a lesson's.
//
// ⛔ THE BODY IS THE POINT, SO THE BOUND IS GENEROUS RATHER THAN TIDY. plan/09
// B60's finding is that `record put` took its body through argv only and "a
// working note is PROSE: the one input path is the one that cannot carry it".
// 64 KiB is well past any single thought and well short of anything that
// should be a file.
const MaxBody = 64 << 10

// DefaultLimit is how many notes a read answers when the caller names none.
//
// ⛔ A DEFAULT RATHER THAN "ALL", because A6's done-when is that an agent gets
// its notes back "without reading everything". A read that defaulted to every
// note would hand a resuming agent its whole history and spend the context the
// notes exist to save. Recent.Total says how many there really are, so the
// caller learns what it did not get.
const DefaultLimit = 20

// MaxLimit is the most notes one read will answer.
const MaxLimit = 200

// The two refusals that carry no value, as sentinels so a caller can match
// them rather than the prose.
var (
	// ⛔ A NOTE WITH NO PROSE IS NOTHING. The body is the whole point of a
	// working note (plan/09: "a working note is PROSE"), so an empty one is a
	// caller bug rather than an empty note.
	errNoBody = errors.New("worknote: a note needs prose; its body is what it is FOR")

	// ⛔ REFUSED RATHER THAN SKIPPED, exactly as record.Append refuses an
	// empty attachment id: it is what a shell variable that expanded to
	// nothing looks like, and skipping it writes the note with one fewer tag
	// than the caller asked for while reporting nothing wrong.
	errEmptyTag = errors.New("worknote: an empty tag is what a variable that expanded to nothing looks like")
)

// Note is one working note as this package answers it.
//
// ⛔ Tags COMES BACK AS A LIST EVEN THOUGH IT IS STORED AS FIELDS. The encoding
// is this package's business and no caller above it should have to know the
// `tag:` prefix to read back what it wrote.
type Note struct {
	ID      string
	Project string
	Body    string
	Tags    []string

	// Fields are the note's OTHER fields, with the tag encoding removed. An
	// agent may put anything here beside its prose; `title` is the one the
	// rest of rig already renders.
	Fields map[string]string

	// Attached are the ids this note was attached to, and Missing the ones
	// that held no record. Both are empty on a read: they are an account of
	// what one WRITE did, and only a write can have one.
	Attached []string
	Missing  []string

	Seat    string
	Session string
	Epoch   uint64
	Written string
}

// Notes is a bounded answer with the unbounded count beside it.
type Notes struct {
	Notes []Note

	// Total is how many match with no limit at all. IT IS NOT len(Notes), and
	// that is the field's reason to exist: an answer that is cut without
	// saying so is indistinguishable from a complete one.
	Total uint64
}

// More reports whether the answer was cut.
func (n Notes) More() bool { return n.Total > uint64(len(n.Notes)) }

// WriteRequest is one note, as an agent writes it.
type WriteRequest struct {
	// Project is which project's record it lands in. Required, like every
	// other record's.
	Project string

	// Body is the prose. Required: a note with no prose is nothing.
	Body string

	// Tags are A3's, any words the agent likes. Optional.
	Tags []string

	// Fields ride beside the prose. Optional.
	//
	// ⛔ A KEY THAT LOOKS LIKE A TAG IS REFUSED RATHER THAN STORED. `tag:x`
	// arriving through Fields would make Tags and Fields disagree about the
	// same row, and a caller reading the note back could not tell which one
	// the store believes. It is the same argument record.Append makes about
	// an empty attachment id: a silent narrowing is the failure.
	Fields map[string]string

	// PartOf is A2: "a work-item, a project, a task, anything it wants".
	// Optional, and an id that holds no record lands in Missing rather than
	// failing the write.
	PartOf []string

	// Session, Seat and Epoch are the DAEMON's, read off the connection's
	// occupancy and never off a request. The forgery rule verbs.proto states.
	Session string
	Seat    string
	Epoch   uint64
}

// Write appends one working note.
//
// ⛔ A STREAM OF SMALL RECORDS AND NOT A GROWING DOCUMENT, WHICH plan/09
// DEMANDS BE DECIDED BEFORE BUILDING: "the two answer 'what did I think at
// 14:00' completely differently". An append answers it directly - the note IS
// the moment, with its own stamp - and costs no rewrite. A growing body mints
// a version per edit, which is the cost the same paragraph names, and it makes
// the 14:00 question a diff of two versions rather than a row.
func Write(ctx context.Context, s *record.Store, r WriteRequest) (Note, error) {
	if strings.TrimSpace(r.Body) == "" {
		return Note{}, errNoBody
	}
	if len(r.Body) > MaxBody {
		return Note{}, fmt.Errorf("worknote: a body is at most %d bytes and this one is %d", MaxBody, len(r.Body))
	}
	tags, err := cleanTags(r.Tags)
	if err != nil {
		return Note{}, err
	}
	fields, err := encode(r.Fields, tags)
	if err != nil {
		return Note{}, err
	}

	got, err := s.Append(ctx, record.AppendRequest{
		Kind: Kind, Project: r.Project, Body: r.Body, Fields: fields,
		PartOf:  r.PartOf,
		Session: r.Session, Seat: r.Seat, Epoch: r.Epoch,
	})
	if err != nil {
		return Note{}, err
	}
	out := noteOf(got.Record)
	out.Attached, out.Missing = got.Attached, got.Missing
	return out, nil
}

// MineRequest asks for the notes one seat wrote.
type MineRequest struct {
	// Seat is whose notes. Required, and the DAEMON's: an empty one is a
	// refusal rather than "every seat", so one agent can never be handed
	// another's notes by an argument that went missing.
	Seat string

	// Project narrows to one project. Empty means every project, which is the
	// right default on resume: an agent does not always know which project its
	// last occupancy was working in.
	Project string

	// Limit bounds the notes and never the count. Zero means DefaultLimit.
	Limit int
}

// Mine answers A6: the notes this SEAT wrote, newest first.
//
// ⛔ THE SEAT AND NOT THE SESSION, WHICH IS THE WHOLE POINT OF COMING BACK. A
// session dies with its process, so a session-keyed read after a kill always
// answers nothing. The seat is section 16's addressable identity and outlives
// every occupancy of it, which is what makes "nothing is ever lost" survive
// the death this feature is named for.
func Mine(ctx context.Context, s *record.Store, r MineRequest) (Notes, error) {
	got, err := s.FindBySeat(ctx, record.SeatFilter{
		Seat: r.Seat, Kind: Kind, Project: r.Project, Limit: limitOf(r.Limit),
	})
	if err != nil {
		return Notes{}, err
	}
	return notesOf(got), nil
}

// AboutRequest asks for the notes attached to one record.
type AboutRequest struct {
	// ID is the record the notes are attached to. Required.
	ID string

	// Limit bounds the notes and never the count. Zero means DefaultLimit.
	Limit int
}

// About answers A2 read backwards: every agent's notes on one thing, newest
// first, whoever wrote them.
//
// ⛔ IT IS NOT FILTERED BY SEAT AND THAT IS DELIBERATE. Mine is one agent's
// memory; About is what the ESTATE knows about a work item, which is the
// question a second agent picking the item up actually has. A seat filter here
// would make a handover impossible through the same door that made the notes
// durable.
func About(ctx context.Context, s *record.Store, r AboutRequest) (Notes, error) {
	got, err := s.FindAttachedTo(ctx, record.AttachedFilter{
		To: r.ID, Type: record.LinkPartOf, Kind: Kind, Limit: limitOf(r.Limit),
	})
	if err != nil {
		return Notes{}, err
	}
	return notesOf(got), nil
}

// limitOf applies the default and the ceiling.
//
// ⛔ A LIMIT OVER THE CEILING IS CLAMPED RATHER THAN REFUSED, which is the
// opposite of record.MaxAttachments and the difference is which side loses. An
// over-large attachment list is refused because truncating it would silently
// drop an association the caller asked for; an over-large limit is clamped
// because Total tells the caller exactly how much it did not get, so nothing
// is silent and a refusal would only cost it a round trip.
func limitOf(n int) int {
	switch {
	case n <= 0:
		return DefaultLimit
	case n > MaxLimit:
		return MaxLimit
	default:
		return n
	}
}

// cleanTags validates A3's tags and returns them deduplicated, in order.
//
// ⛔ "ANY TAGS IT WANTS" IS A3, SO THE VALIDATION IS SHAPE AND NEVER
// VOCABULARY. Nothing here decides which tags are allowed - there is no list,
// no namespace and no registry. What it refuses is a tag that could not be
// queried back: an empty one, one with whitespace in it (the CLI and the
// predicate both split on it), and one long enough to be prose.
func cleanTags(in []string) ([]string, error) {
	if len(in) > MaxTags {
		return nil, fmt.Errorf("worknote: a note carries at most %d tags and this one has %d", MaxTags, len(in))
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, t := range in {
		switch {
		case strings.TrimSpace(t) == "":
			return nil, errEmptyTag
		case len(t) > MaxTagLen:
			return nil, fmt.Errorf("worknote: %q is %d bytes; a tag is at most %d and prose belongs in the body", t, len(t), MaxTagLen)
		case strings.IndexFunc(t, unicode.IsSpace) >= 0:
			return nil, fmt.Errorf("worknote: tag %q has whitespace in it, so it could never be selected back out", t)
		}
		if seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out, nil
}

// encode folds the tags into the fields map the store writes.
func encode(fields map[string]string, tags []string) (map[string]string, error) {
	out := make(map[string]string, len(fields)+len(tags))
	for k, v := range fields {
		if strings.HasPrefix(k, TagPrefix) {
			return nil, fmt.Errorf(
				"worknote: field %q is a tag; pass it as a tag so the note has one answer to what it is tagged", k)
		}
		out[k] = v
	}
	for _, t := range tags {
		out[TagPrefix+t] = TagValue
	}
	return out, nil
}

// noteOf decodes one stored record back into a note.
func noteOf(r record.Record) Note {
	n := Note{
		ID: r.ID, Project: r.Project, Body: r.Body,
		Fields:  make(map[string]string, len(r.Fields)),
		Tags:    make([]string, 0, len(r.Fields)),
		Seat:    r.Prov.Seat,
		Session: r.Prov.Session,
		Epoch:   r.Prov.Epoch,
		Written: r.Prov.CreatedAt.UTC().Format(stamp),
	}
	for k, v := range r.Fields {
		if t, ok := strings.CutPrefix(k, TagPrefix); ok {
			n.Tags = append(n.Tags, t)
			continue
		}
		n.Fields[k] = v
	}
	// ⛔ SORTED, BECAUSE A MAP'S ORDER IS RANDOM PER ITERATION IN Go. Without
	// this the same note answers a different tag order on every read, which
	// makes a golden test flap and a diff of two reads meaningless. The
	// caller's original order is not recoverable from the encoding and is not
	// worth a second field: a tag set is a set.
	sort.Strings(n.Tags)
	return n
}

// stamp is the note's written time, to the second, in UTC.
//
// RFC3339 rather than the record's raw nanoseconds because a note is read by a
// person as often as by an agent, and "what did I think at 14:00" is a
// question asked in wall-clock terms.
const stamp = "2006-01-02T15:04:05Z"

func notesOf(r record.Recent) Notes {
	out := Notes{Notes: make([]Note, 0, len(r.Records)), Total: r.Total}
	for _, rec := range r.Records {
		out.Notes = append(out.Notes, noteOf(rec))
	}
	return out
}
