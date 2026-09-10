// Command ledger is M1a's first fake application.
//
// PLAN.md section 5h: "A fake application that is written to fit the kit
// proves nothing", so each one is modelled on a real program's measured markup
// and the kit is what has to bend. This one is modelled on `pull-report`, which
// the census puts at 18 tables and direct measurement at 16, every one of them
// in the same five-part shape: a heading, a real paragraph of lead, a toolbar
// with a search and a live count, and a table inside a bounded scroll box that
// is EMPTY in the markup and filled by script.
//
// It is not `cmd/fakeapp`, which is the conformance suite's reference program
// and shares nothing with this but a word. This one exists to use the shell and
// to shake out the element list (R3), which is why it serves its own HTML.
//
// What it serves, and why it serves it rather than rig serving it: rig has no
// HTTP server until M2, so it cannot publish the kit today. rig publishes the
// kit as source in design/kit/, the program embeds it and serves it beside its
// own page, and that is the same file a real adopter would vendor at M8.
package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/client"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// The `go:embed` directive cannot reach above its own package, exactly as it
// cannot for the window's built frontend, so `make build-ledger` copies
// design/kit in here first and kit/.gitkeep is what keeps this embed
// resolvable on a fresh clone. The `all:` prefix makes .gitkeep count as a
// match. (Written with backticks: as bare prose, the space after the slashes
// reads to staticcheck as a broken compiler directive.)
//
//go:embed all:kit
var kitFS embed.FS

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ledger: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	id := flag.String("name", "ledger", "the program id to announce")
	addr := flag.String("addr", "127.0.0.1:7451", "where to serve its own pane")
	flag.Parse()

	origin := "http://" + *addr

	mux := http.NewServeMux()
	mux.HandleFunc("/pane", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/html; charset=utf-8")
		fmt.Fprint(w, pane())
	})

	// Served from the embed rather than from disk, because a real adopter has
	// the vendored file and not this repo.
	sub, err := fs.Sub(kitFS, "kit")
	if err != nil {
		return err
	}
	mux.Handle("/kit/", http.StripPrefix("/kit/", http.FileServer(http.FS(sub))))

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	errs := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errs <- err
		}
	}()

	// internal/paths is not importable from outside the module and a program is
	// outside it by definition, so the convention is spelled out rather than
	// borrowed. A fake application that reached into rig's internals would be
	// testing something no real adopter can do.
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		return errors.New("XDG_RUNTIME_DIR is unset, so rig's socket cannot be found")
	}
	sock := filepath.Join(runtimeDir, "rig", "rigd.sock")

	c, err := client.Dial(sock)
	if err != nil {
		return err
	}
	defer c.Close()

	// The method is not read: this program declares one command and answers the
	// probe, so there is nothing to route on yet.
	c.Handle(func(_ string, payload []byte) (proto.Message, error) {
		var req rigv1.PingRequest
		if err := proto.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		return &rigv1.PingResponse{
			Nonce:   req.GetNonce(),
			Program: *id,
			Version: version,
		}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Hello(ctx, declaration(*id, origin+"/pane"))
	if err != nil {
		return err
	}
	fmt.Printf("%s up: pane %s, wire %s, rigd %s\n",
		*id, origin+"/pane", resp.GetWire(), resp.GetDaemonVersion())

	select {
	case err := <-errs:
		return err
	case <-c.Done():
		return c.Err()
	}
}

// declaration says what this program is, and the only field here that step 3
// is about is PaneUrl: non-empty is what moves it off the generated tier.
//
// Coverage is partial and honestly so (section 5k: no surface may imply
// completeness). It adopts the wire and the pane, and nothing else.
func declaration(id, paneURL string) *rigv1.Declaration {
	return &rigv1.Declaration{
		Identity: &rigv1.Identity{
			Id:          id,
			Name:        "Ledger",
			Version:     version,
			Description: "A pull report, modelled on pull-report's measured markup.",
		},
		Coverage:     rigv1.Coverage_COVERAGE_PARTIAL,
		CoverageNote: "the wire and its own pane: no config, no storage, no logs",
		SemanticsGen: 1,
		PaneUrl:      paneURL,

		// R3: the element list is a REGISTRATION declaration, so this is where
		// it is said. R7 then refuses the handshake on a name rig does not
		// serve, which means a page rig cannot draw stops the program at
		// startup rather than at render.
		//
		// These three are what page.go actually imports, and
		// TestTheDeclaredElementsAreTheOnesThePageImports is what keeps that
		// true: a declaration that drifts from the page it describes is the
		// exact failure R3 centralises the check to avoid.
		Elements: []string{"rigPanel", "rigTable", "rigToolbar"},
		Commands: []*rigv1.Command{{
			Id:           "ping",
			Title:        "Ping",
			Effects:      rigv1.Effects_EFFECTS_READ_ONLY,
			Idempotent:   rigv1.Tristate_TRISTATE_YES,
			Sensitive:    &rigv1.SensitiveFields{},
			Interactive:  rigv1.Tristate_TRISTATE_NO,
			Streams:      rigv1.Tristate_TRISTATE_NO,
			NeedsDisplay: rigv1.Tristate_TRISTATE_NO,
			Duration:     rigv1.Duration_DURATION_INSTANT,
			Confirms:     rigv1.Tristate_TRISTATE_NO,
			Shape:        rigv1.Shape_SHAPE_UNARY,
			Summary:      "Round-trip a nonce",
			Description:  "Echoes the nonce it was given, so a caller can prove the round trip was its own.",
			Returns:      "The nonce, this program's id and its version.",
		}},
	}
}
