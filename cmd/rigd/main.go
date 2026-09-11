// Command rigd is the daemon.
//
// Two binaries, and this is the one that links none of the terminal stack
// (PLAN.md section 22): no bubbletea, no huh, no glamour, no lipgloss. The
// split is what makes `make bench-size` able to attribute a dependency's cost
// to the process that actually pays it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/boris-milner/rig/internal/daemon"
	"github.com/boris-milner/rig/internal/instance"
	"github.com/boris-milner/rig/internal/paths"
)

// Set by the Makefile's ldflags.
var (
	version = "dev"
	wire    = "v1"
	sha     = "none"
	date    = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "rigd: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	level := flag.String("log-level", "info", "debug | info | warn | error")
	showVersion := flag.Bool("version", false, "print every version this build carries and exit")
	estate := flag.String("estate", "", "name this estate (PLAN.md section 37); "+
		"unnamed estates claim no name and collide with nothing")
	flag.Parse()

	if *showVersion {
		fmt.Printf("product %s\nwire    %s\ncommit  %s\nbuilt   %s\n", version, wire, sha, date)
		return nil
	}

	var lv slog.Level
	if err := lv.UnmarshalText([]byte(*level)); err != nil {
		return fmt.Errorf("log level %q: %w", *level, err)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lv}))

	pidPath, err := paths.PIDFile()
	if err != nil {
		return err
	}
	sockPath, err := paths.Socket()
	if err != nil {
		return err
	}
	mcpSockPath, err := paths.MCPSocket()
	if err != nil {
		return err
	}

	// The lock comes BEFORE the bind, and that order is the whole point
	// (section 5f). Two daemons over one state tree give two serialisation
	// points, two WALs and two lock namespaces, silently - and every property
	// section 16 proves is false for as long as it lasts.
	lock, err := instance.Acquire(pidPath)
	if err != nil {
		var held *instance.HeldError
		if errors.As(err, &held) {
			// Exit with the incumbent named, rather than unlinking the socket
			// and taking over.
			return errors.New(held.Error())
		}
		return err
	}
	defer func() { _ = lock.Close() }()

	// A NAMED estate takes a SECOND claim, and it is taken HERE - beside the
	// directory lock and before the first os.Remove below - rather than
	// anywhere later (section 37, precondition 6).
	//
	// THE ORDER IS THE MECHANISM, not tidiness. The directory lock cannot see a
	// second estate at all, because two estates differ exactly in their runtime
	// directory. If this claim were taken after the removals, a second estate
	// calling itself `production` would unlink the incumbent's sockets, THEN
	// discover the name was held, and exit with the right message and the right
	// code having already broken a live estate - and this function's defers do
	// not put another process's sockets back.
	//
	// An estate started without --estate claims nothing and reaches none of
	// this, which is how every test in this repository keeps working by
	// construction rather than by exemption.
	if *estate != "" {
		claimPath, err := paths.EstateLock(*estate)
		if err != nil {
			return err
		}
		nameClaim, err := instance.AcquireName(claimPath, *estate)
		if err != nil {
			var held *instance.NameHeldError
			if errors.As(err, &held) {
				return errors.New(held.Error())
			}
			return err
		}
		defer func() { _ = nameClaim.Close() }()
		log.Info("estate named", "estate", *estate, "claim", claimPath)
	}

	// Only now, holding the lock, is a stale socket ours to remove. Doing this
	// before the lock is how a second daemon steals a live one's socket.
	if err := os.Remove(sockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing stale socket: %w", err)
	}
	//nolint:noctx // Binding a unix socket is a filesystem operation. A
	// ListenConfig's context cancels a pending network bind, of which there
	// is none here.
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen %s: %w", sockPath, err)
	}
	defer func() { _ = l.Close(); _ = os.Remove(sockPath) }()

	// 0600 explicitly. Listen's mode depends on the process umask, and a
	// socket whose permissions vary with an inherited umask is not a boundary.
	if err := os.Chmod(sockPath, 0o600); err != nil {
		return fmt.Errorf("chmod socket: %w", err)
	}

	// The MCP surface gets its own socket (PLAN.md sections 9, 14). It is
	// bound under the same lock, with the same mode, in the same 0700 runtime
	// directory - so it reaches exactly the processes the first socket
	// reaches and widens nothing. What it buys is a caller row of its own,
	// which neither stdio nor the HTTP surface could give it.
	if err := os.Remove(mcpSockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing stale mcp socket: %w", err)
	}
	//nolint:noctx // Binding a unix socket is a filesystem operation, as above.
	ml, err := net.Listen("unix", mcpSockPath)
	if err != nil {
		return fmt.Errorf("listen %s: %w", mcpSockPath, err)
	}
	defer func() { _ = ml.Close(); _ = os.Remove(mcpSockPath) }()
	if err := os.Chmod(mcpSockPath, 0o600); err != nil {
		return fmt.Errorf("chmod mcp socket: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("rigd up", "version", version, "wire", wire,
		"socket", sockPath, "mcp", mcpSockPath, "pid", os.Getpid())

	// Config carries the lock, so this cannot compile without having taken it.
	d, err := daemon.New(daemon.Config{
		Version: version,
		Wire:    wire,
		Log:     log,
		Lock:    lock,
	})
	if err != nil {
		return err
	}
	// The MCP surface runs beside the main one, on its own context, so that
	// whatever ends Serve - a signal or rig.down, which cancels a context
	// only Serve holds - ends this too. Without the separate cancel, rig.down
	// would stop answering the wire and leave agents connected.
	//rig:allow nocontextfree: this context bounds the MCP surface's whole serving life, so being unbounded is the requirement rather than an oversight
	mcpCtx, stopMCP := context.WithCancel(ctx)
	defer stopMCP()
	mcpDone := make(chan struct{})
	go func() {
		defer close(mcpDone)
		if err := d.ServeMCP(mcpCtx, ml); err != nil {
			log.Error("the mcp surface stopped", "err", err)
		}
	}()

	if err := d.Serve(ctx, l); err != nil {
		return err
	}
	stopMCP()
	<-mcpDone
	log.Info("rigd down")
	return nil
}
