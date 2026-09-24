package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// `rig knowledge` - section 40's lessons at the prompt.
//
//	rig knowledge search <words...> [--limit N]   titles, summaries, snippets
//	rig knowledge get <id>                        one lesson whole
//	rig knowledge add --title T --summary S [--body B] [--tag t]...
//
// search never prints a body: that is section 40's whole point, and the
// reason `get` exists. Every form takes --json and --timeout.

// tagList is a repeatable --tag.
type tagList []string

func (t *tagList) String() string     { return strings.Join(*t, ",") }
func (t *tagList) Set(v string) error { *t = append(*t, v); return nil }

type knowledgeFlags struct {
	fs                   *flag.FlagSet
	asJSON               *bool
	timeout              *time.Duration
	limit                *uint
	title, summary, body *string
	tags                 tagList
}

func knowledgeFlagSet() *knowledgeFlags {
	k := &knowledgeFlags{fs: flag.NewFlagSet("knowledge", flag.ContinueOnError)}
	k.asJSON = k.fs.Bool("json", false, "emit JSON")
	k.timeout = k.fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	k.limit = k.fs.Uint("limit", 0, "search: at most this many hits (default 5, at most 20)")
	k.title = k.fs.String("title", "", "add: the lesson's title")
	k.summary = k.fs.String("summary", "", "add: one line, what a search shows")
	k.body = k.fs.String("body", "", "add: the detail")
	k.fs.Var(&k.tags, "tag", "add: a one-word tag (repeatable)")
	return k
}

func cmdKnowledge(args []string) (err error) {
	k := knowledgeFlagSet()
	flags, positional := partition(args)
	if err := k.fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *k.asJSON) }()
	usage := "usage: rig knowledge search <words...> [--limit N] | get <id> | " +
		"add --title T --summary S [--body B] [--tag t]..."
	if len(positional) == 0 {
		return badArgumentf("%s", usage)
	}
	sub, rest := positional[0], positional[1:]
	switch {
	case sub == "search" && len(rest) > 0:
	case sub == "get" && len(rest) == 1:
	case sub == "add" && len(rest) == 0:
	default:
		return badArgumentf("%s", usage)
	}

	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *k.timeout)
	defer cancel()
	out := json.NewEncoder(os.Stdout)

	switch sub {
	case "search":
		var resp verbsv1.KnowledgeSearchResponse
		if err := call(ctx, c, "rig.knowledge.search", &verbsv1.KnowledgeSearchRequest{
			Query: strings.Join(rest, " "), Limit: uint32(min(*k.limit, 1<<16)),
		}, &resp); err != nil {
			return err
		}
		if *k.asJSON {
			hits := make([]map[string]any, 0, len(resp.GetHits()))
			for _, h := range resp.GetHits() {
				hits = append(hits, map[string]any{
					"id": h.GetId(), titleKey: h.GetTitle(), "summary": h.GetSummary(),
					"snippet": h.GetSnippet(), "score": h.GetScore(),
				})
			}
			return out.Encode(map[string]any{"hits": hits})
		}
		if len(resp.GetHits()) == 0 {
			fmt.Println("no lesson matches. Nothing is recorded on this yet; if you learn it, `rig knowledge add` it.")
			return nil
		}
		for _, h := range resp.GetHits() {
			fmt.Printf("%s  %s\n    %s\n    ...%s\n", h.GetId(), h.GetTitle(), h.GetSummary(), h.GetSnippet())
		}
		fmt.Println("\n`rig knowledge get <id>` reads one whole.")
		return nil
	case "get":
		var resp verbsv1.KnowledgeGetResponse
		if err := call(ctx, c, "rig.knowledge.get", &verbsv1.KnowledgeGetRequest{Id: rest[0]}, &resp); err != nil {
			return err
		}
		return printLesson(out, resp.GetLesson(), *k.asJSON)
	default:
		var resp verbsv1.KnowledgeAddResponse
		if err := call(ctx, c, "rig.knowledge.add", &verbsv1.KnowledgeAddRequest{
			Title: *k.title, Summary: *k.summary, Body: *k.body, Tags: k.tags,
		}, &resp); err != nil {
			return err
		}
		return printLesson(out, resp.GetLesson(), *k.asJSON)
	}
}

func printLesson(out *json.Encoder, l *verbsv1.Lesson, asJSON bool) error {
	if asJSON {
		tags := l.GetTags()
		if tags == nil {
			tags = []string{}
		}
		return out.Encode(map[string]any{
			"id": l.GetId(), titleKey: l.GetTitle(), "summary": l.GetSummary(),
			"body": l.GetBody(), "tags": tags, "seat": l.GetProv().GetSeat(),
			epochKey: l.GetProv().GetEpoch(), "at_unix_nano": l.GetProv().GetAtUnixNano(),
		})
	}
	fmt.Printf("%s  %s\n%s\n", l.GetId(), l.GetTitle(), l.GetSummary())
	if len(l.GetTags()) > 0 {
		fmt.Printf("tags: %s\n", strings.Join(l.GetTags(), " "))
	}
	fmt.Printf("from %s\n\n%s\n", l.GetProv().GetSeat(), l.GetBody())
	return nil
}
