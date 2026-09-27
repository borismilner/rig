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
	"path/filepath"
	"syscall"

	"github.com/borismilner/rig/internal/audio"
	"github.com/borismilner/rig/internal/coord"
	"github.com/borismilner/rig/internal/daemon"
	"github.com/borismilner/rig/internal/instance"
	"github.com/borismilner/rig/internal/paths"
	"github.com/borismilner/rig/internal/supervise"
)

// Set by the Makefile's ldflags.
var (
	version = "dev"
	wire    = "v1"
	sha     = "none"
	date    = "unknown"
)

// exitAlreadyRunning is the status rigd exits with when it refuses because
// another daemon already holds this runtime directory or this estate name.
//
// IT IS DISTINCT FROM 1 ON PURPOSE, AND THE REASON WAS MEASURED HERE RATHER
// THAN REASONED ABOUT. systemd's Restart=on-failure treats every non-zero
// status as a fault worth retrying, so a refusal that shares 1 with a real
// crash is retried against a healthy incumbent forever: `systemctl --user
// start rigd` against a running daemon produced 14 restarts in 28 seconds,
// still climbing, roughly five journal lines each. The unit's own comment
// called this "one wrinkle, inherited and accepted" and described the unit as
// reporting inactive. It does not; it reports activating (auto-restart) and
// never stops.
//
// The header above already records the opposite defect: agentbox exited 0 in
// this case, systemd read that as the service finishing, and ExecStop killed
// the healthy daemon. Neither 0 nor 1 is right on its own. A THIRD status is,
// because it lets the unit say "this one is not a fault to retry" without
// claiming the daemon started.
//
// packaging/rigd.service names this number in RestartPreventExitStatus, and
// TestTheUnitDoesNotRetryARefusal ties the two together so they cannot drift.
//
// WHY 8 AND NOT 3, WHICH IS WHAT THIS WAS FIRST: systemd prints a NAME beside
// the status, and it takes that name from the LSB table. Measured on this
// machine - 3 prints as `status=3/NOTIMPLEMENTED`, sitting one line under
// "another rigd is running", which invites a reader to conclude a feature is
// missing. 8 is the first code above the LSB range and prints as
// `status=8/n/a`, which invents nothing and leaves rigd's own message as the
// only explanation. 69 and 75 were also measured and both carry a name that
// is wrong here: UNAVAILABLE says the service is not there when it is, and
// TEMPFAIL invites the retry this constant exists to prevent.
const exitAlreadyRunning = 8

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "rigd: "+err.Error())
		os.Exit(exitStatus(err))
	}
}

// exitStatus maps a refusal by an incumbent onto its own status, and
// everything else onto 1.
//
// It matches on TYPE rather than on the message, because the message names a
// pid and a path and is meant to stay readable to a person.
func exitStatus(err error) int {
	var held *instance.HeldError
	var named *instance.NameHeldError
	var rooted *RootHeldError
	if errors.As(err, &held) || errors.As(err, &named) || errors.As(err, &rooted) {
		return exitAlreadyRunning
	}
	return 1
}

func run() error {
	level := flag.String("log-level", "info", "debug | info | warn | error")
	showVersion := flag.Bool("version", false, "print every version this build carries and exit")
	estate := flag.String("estate", "", "name this estate (PLAN.md section 37); "+
		"unnamed estates claim no name and collide with nothing")
	rootFlag := flag.String("root", "", "where the estate keeps its storage (plan/48); "+
		"default "+paths.RootEnv+", else ~/.rig. An unnamed estate uses the runtime directory unless told")
	filesEvery := flag.Duration("files-commit-every", daemon.DefaultFilesCommitEvery,
		"the least time between two commits of the free files (plan/48 R34); text files only")
	soundFile := flag.String("toast-sound-file", "", "an absolute path to a sound a toast may play "+
		"instead of rig's own (plan/12); chosen with `rig sound use file` or the tray")
	flag.Parse()

	root, err := resolveRoot(*estate, *rootFlag)
	if err != nil {
		return err
	}

	if *showVersion {
		fmt.Printf("product %s\nwire    %s\ncommit  %s\nbuilt   %s\n", version, wire, sha, date)
		return nil
	}

	var lv slog.Level
	if err := lv.UnmarshalText([]byte(*level)); err != nil {
		return fmt.Errorf("log level %q: %w", *level, err)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lv}))
	log.Info("storage root", "root", root)

	pidPath, sockPath, mcpSockPath, err := runtimePaths()
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
			//
			// THE TYPE IS RETURNED, NOT ITS MESSAGE. errors.New(held.Error())
			// read identically to a person and flattened the one thing main
			// needs to pick an exit status by.
			return held
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
	// THE EPOCH IS DECLARED OUT HERE, not inside the branch that sets it,
	// because the daemon below needs it and an unnamed estate has none. Zero
	// is the honest value for that case rather than a hole: an unnamed estate
	// opens no store, so it has no epoch, and a real epoch is always at least
	// 1 because the store bumps before it publishes.
	var epoch uint64
	var leases *coord.Store

	// The storage root's claim, before the estate's state is opened, so a
	// daemon refused here bumps no epoch.
	rootClaim, err := claimRoot(*estate, root)
	if err != nil {
		return err
	}
	defer func() { _ = rootClaim.Close() }()

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
				// The type, not its message - same reason as the directory
				// lock above.
				return held
			}
			return err
		}
		defer func() { _ = nameClaim.Close() }()
		log.Info("estate named", "estate", *estate, "claim", claimPath)

		// THE ESTATE'S PERSISTENT STATE, opened here and for the same reason
		// the claim is taken here: under the lock, before anything binds
		// (section 37, preconditions 2 and 4). Two daemons must never have
		// this file open at once, and the claim above is what guarantees it -
		// bbolt's own file lock would too, but it would report the collision
		// as a three-second timeout rather than as "that name is taken".
		//
		// AN UNNAMED ESTATE OPENS NOTHING, which is why this sits inside the
		// named branch rather than beside it. It has no name to key a subtree
		// to, so it has no persistent state at all; every test in this
		// repository starts one, and that is what keeps them from sharing a
		// store with the developer's live estate.
		//
		// OPENING IT IS WHAT BUMPS THE EPOCH, unconditionally, once per start
		// (section 37, precondition 4 and V15). Nothing distinguishes a
		// planned restart from a crash here, deliberately: the cost of
		// treating a restart as a crash is one re-acquisition, and the cost of
		// the reverse is a fencing token that outlives what it fences.
		st, err := coord.Open(*estate)
		if err != nil {
			return err
		}
		defer func() { _ = st.Close() }()
		epoch = st.Epoch()
		leases = st
		log.Info("estate state opened",
			"estate", *estate,
			"path", st.Path(),
			"epoch", epoch,
			"rebooted", st.Rebooted())
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

	sup := supervisor(log)

	sounds, err := newAudio(log, *estate, pidPath, *soundFile)
	if err != nil {
		return err
	}
	defer sounds.Close()

	// Config carries the lock, so this cannot compile without having taken it.
	d, err := daemon.New(daemon.Config{
		Version: version,
		Wire:    wire,
		Estate:  *estate,
		Epoch:   epoch,
		Root:    root,

		FilesCommitEvery: *filesEvery,
		Log:              log,
		Lock:             lock,
		Leases:           leases,

		Supervisor: sup,
		Audio:      sounds,
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

	// The supervisor stops with Serve for the MCP surface's reason, and its
	// Run stops every child it started on the way out: section 18, "programs
	// rig did not start are never killed", and the ones it did start do not
	// outlive it.
	//rig:allow nocontextfree: this context bounds the supervisor's whole life, as the MCP surface's does
	supCtx, stopSup := context.WithCancel(ctx)
	defer stopSup()
	supDone := make(chan struct{})
	go func() { defer close(supDone); sup.Run(supCtx) }()

	if err := d.Serve(ctx, l); err != nil {
		stopSup()
		<-supDone
		return err
	}
	stopSup()
	<-supDone
	stopMCP()
	<-mcpDone
	closeDaemon(d, log)
	log.Info("rigd down")
	return nil
}

// newAudio builds section 12's audio queue (plan/12, S1-S5). The settings
// are kept in the estate's state directory, so they survive a restart; an
// unnamed estate keeps them beside its pidfile, in the runtime directory,
// which a reboot clears. The synthesised sounds go to the user's cache.
func newAudio(log *slog.Logger, estate, pidPath, soundFile string) (*audio.Audio, error) {
	dir := filepath.Dir(pidPath)
	if estate != "" {
		d, err := paths.EstateStateDir(estate)
		if err != nil {
			return nil, err
		}
		dir = d
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("the sounds need a cache directory: %w", err)
	}
	return audio.New(audio.Options{
		Log:          log.With("component", "audio"),
		SettingsPath: filepath.Join(dir, "sound.json"),
		SoundDir:     filepath.Join(cache, "rig", "sounds"),
		SoundFile:    soundFile,
	})
}

// supervisor builds section 18's supervisor over programs.json.
//
// ⛔ A BAD programs.json IS LOGGED, NOT FATAL. Refusing to start would take
// the whole estate down over one typo in a file that only lists what MAY run;
// the file's error is in the log and rig health answers with nothing
// declared, which is visibly wrong rather than silently so.
func supervisor(log *slog.Logger) *supervise.Supervisor {
	sup := supervise.New(supervise.Options{})
	specs, err := supervise.LoadDefault()
	if err == nil {
		err = sup.Declare(specs)
	}
	if err != nil {
		log.Error("no programs are supervised: programs.json was refused", "err", err)
		return sup
	}
	if len(specs) > 0 {
		log.Info("programs declared", "count", len(specs), "ids", sup.Declared())
	}
	return sup
}

// closeDaemon releases the daemon's record store once both surfaces have
// stopped. A failure is logged rather than returned: the daemon is already
// down, and the exit status belongs to why it stopped.
func closeDaemon(d *daemon.Daemon, log *slog.Logger) {
	if err := d.Close(); err != nil {
		log.Warn("closing the record store", "err", err)
	}
}

// runtimePaths are the pidfile and the two sockets in the runtime directory.
func runtimePaths() (pid, sock, mcpSock string, err error) {
	if pid, err = paths.PIDFile(); err != nil {
		return "", "", "", err
	}
	if sock, err = paths.Socket(); err != nil {
		return "", "", "", err
	}
	if mcpSock, err = paths.MCPSocket(); err != nil {
		return "", "", "", err
	}
	return pid, sock, mcpSock, nil
}

// resolveRoot picks the storage root (plan/48, R14-R15). A named estate reads
// --root, then RIG_ROOT, then ~/.rig; an unnamed one reads only --root, so the
// environment can never point a disposable estate at the live store.
//
// It checks the estate name first, because a named estate's root is keyed by
// that name, and one helper keeps run under gocyclo's ceiling.
func resolveRoot(estate, flagValue string) (string, error) {
	if err := checkEstateName(estate); err != nil {
		return "", err
	}
	if estate == "" {
		return paths.ScratchRoot(flagValue)
	}
	return paths.Root(flagValue)
}
