// Command lantern is the EMBEDDED-tier demo program, the way cmd/fakeapp is
// the generated tier's and cmd/ledger and cmd/abacus are the kit tier's.
//
// Section 11's third tier: "the program serves its own HTML and brings its own
// components, getting the token set and nothing more." So lantern declares a
// pane_url and NO kit elements, serves a page built from its own markup and
// its own CSS, and loads exactly one file of rig's: pane.js, the four-message
// handshake that delivers the token set (design/kit/pane.js: "pane.js alone is
// the embedded tier"). It loads neither kit.css nor kit.js, and a test fails
// if its page ever does.
//
// It is not modelled on a measured program the way ledger and abacus are.
// Section 5h's rule is about the KIT - a fake application written to fit the
// kit proves nothing - and this tier takes no kit to fit. What it has to prove
// is only that a page with none of rig's elements still receives the tokens,
// follows the theme, and says so on screen.
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

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// rigFS holds pane.js, copied from design/kit by `make build-lantern`,
// because an embed cannot reach above its own package. rig/.gitkeep keeps
// the embed resolving on a tree nobody has built.
//
//go:embed all:rig
var rigFS embed.FS

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "lantern: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	id := flag.String("name", "lantern", "the program id to announce")
	addr := flag.String("addr", "127.0.0.1:7453", "where to serve its own pane")
	flag.Parse()

	origin := "http://" + *addr

	mux := http.NewServeMux()
	mux.HandleFunc("/pane", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/html; charset=utf-8")
		fmt.Fprint(w, page)
	})
	sub, err := fs.Sub(rigFS, "rig")
	if err != nil {
		return err
	}
	mux.Handle("/rig/", http.StripPrefix("/rig/", http.FileServer(http.FS(sub))))

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	errs := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errs <- err
		}
	}()

	// Spelled out rather than borrowed from internal/paths, for the reason
	// cmd/abacus gives: a program is outside the module by definition.
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		return errors.New("XDG_RUNTIME_DIR is unset, so rig's socket cannot be found")
	}
	c, err := client.Dial(filepath.Join(runtimeDir, "rig", "rigd.sock"))
	if err != nil {
		return err
	}
	defer c.Close()

	c.Handle(func(_ string, payload []byte) (proto.Message, error) {
		var req rigv1.PingRequest
		if err := proto.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		return &rigv1.PingResponse{Nonce: req.GetNonce(), Program: *id, Version: version}, nil
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

// declaration is the embedded tier in two fields: a PaneUrl, which moves the
// program off the generated tier, and an EMPTY Elements list, which is what
// keeps it off the kit tier.
func declaration(id, paneURL string) *rigv1.Declaration {
	return &rigv1.Declaration{
		Identity: &rigv1.Identity{
			Id:          id,
			Name:        "Lantern",
			Version:     version,
			Description: "The embedded-tier demo: its own page and components, rig's tokens and nothing more.",
		},
		Coverage:     rigv1.Coverage_COVERAGE_PARTIAL,
		CoverageNote: "the wire and its own pane: no config, no storage, no logs",
		SemanticsGen: 1,
		PaneUrl:      paneURL,
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
