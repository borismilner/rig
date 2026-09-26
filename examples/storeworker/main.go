// Command storeworker is a fake program built to use as much of rig as one
// program can, and to talk to its user while it does (plan/48, R37 and R39).
//
// It runs fake assignments the way graft runs real ones: each run is stored,
// queued, claimed by a worker, run under a lease, written up as an indexed
// transcript, and announced by toast and mail. A run over budget parks on a
// question for Boris. Its pane has one tab per part of rig, each saying what
// it shows and letting him try it.
//
// Queues and leases need a named estate, so rigd must run as one
// (`rigd --estate development`).
package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/borismilner/rig/client"
)

// rigFS holds pane.js, copied from design/kit by `make build-storeworker`,
// as cmd/lantern's does. rig/.gitkeep keeps the embed resolving on a tree
// nobody has built.
//
//go:embed all:rig
var rigFS embed.FS

var version = "dev"

func main() {
	if err := serve(); err != nil {
		fmt.Fprintln(os.Stderr, "storeworker: "+err.Error())
		os.Exit(1)
	}
}

func serve() error {
	id := flag.String("name", "storeworker", "the program id to register")
	seat := flag.String("seat", "storeworker", "the seat its worker takes")
	addr := flag.String("addr", "127.0.0.1:7454", "where to serve its pane, on loopback")
	budget := flag.Float64("budget", 1.0, "a run costing more than this, in dollars, waits for an answer")
	step := flag.Duration("step", 1500*time.Millisecond, "how long one simulated step takes")
	flag.Parse()

	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("-addr %s is not a loopback address, and the pane serves nothing else", *addr)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c, err := client.Connect()
	if err != nil {
		return err
	}
	defer c.Close()
	// Boris's own connection: not registered, the way a terminal reaches rig.
	you, err := client.Connect()
	if err != nil {
		return err
	}
	defer you.Close()

	a := newApp(c, *id, *seat, *budget, *step)
	c.Handle(a.handle)

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	sub, err := fs.Sub(rigFS, "rig")
	if err != nil {
		return err
	}
	w := &web{a: a, addr: *addr, you: you}
	srv := &http.Server{Handler: w.handler(http.FS(sub)), ReadHeaderTimeout: 5 * time.Second}
	errs := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()
	defer func() { _ = srv.Close() }()

	hctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	resp, err := c.Hello(hctx, a.declaration("http://"+*addr+"/pane"))
	cancel()
	if err != nil {
		return err
	}
	fmt.Printf("%s up: pane http://%s/pane, wire %s, rigd %s\n", *id, *addr, resp.GetWire(), resp.GetDaemonVersion())

	go a.work(ctx)

	select {
	case err := <-errs:
		return err
	case <-c.Done():
		return c.Err()
	case <-ctx.Done():
		return nil
	}
}
