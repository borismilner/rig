package meta

import (
	"context"
	"errors"

	"github.com/borismilner/rig/internal/kernel"
)

// Lesson is one section 40 lesson, whole, as a tool answers it.
type Lesson struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Summary string   `json:"summary"`
	Body    string   `json:"body"`
	Tags    []string `json:"tags"`
	Seat    string   `json:"seat"`
}

// LessonHit is one search hit: enough to decide whether to fetch it.
type LessonHit struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Summary string  `json:"summary"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

// Lessons is section 40's lessons, as the surface behind the MCP door offers
// them. A Records implementation that also implements it serves the three
// knowledge tools; one that does not answers them as unavailable, the same way
// an estate with no record store does, so an older Records keeps compiling.
type Lessons interface {
	SearchLessons(ctx context.Context, query string, limit int) ([]LessonHit, error)
	GetLesson(ctx context.Context, id string) (Lesson, error)
	AddLesson(ctx context.Context, title, summary, body string, tags []string) (Lesson, error)
}

// knowledge answers knowledge_search, knowledge_get and knowledge_add.
func (s *Server) knowledge(ctx context.Context, who kernel.Principal, r Request) (Answer, error) {
	return s.recordTool(who, r.Tool, func(rc Records, out *Answer) error {
		lc, ok := rc.(Lessons)
		if !ok {
			return errors.New("meta: this estate's store does not serve lessons")
		}
		switch r.Tool {
		case KnowledgeSearchTool:
			hits, err := lc.SearchLessons(ctx, r.Query, r.Limit)
			if err != nil {
				return err
			}
			if hits == nil {
				hits = []LessonHit{}
			}
			out.Record = &RecordAnswer{Hits: hits}
		case KnowledgeGetTool:
			l, err := lc.GetLesson(ctx, r.RecordID)
			if err != nil {
				return err
			}
			out.Record = &RecordAnswer{Lesson: &l}
		default:
			l, err := lc.AddLesson(ctx, r.Title, r.Summary, r.Body, r.Tags)
			if err != nil {
				return err
			}
			out.Record = &RecordAnswer{Lesson: &l}
		}
		return nil
	})
}
