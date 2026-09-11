package daemon

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/boris-milner/rig/internal/mcpserver"
	"github.com/boris-milner/rig/internal/meta"
)

// ServeMCP serves the MCP surface on its own listener until ctx ends.
//
// A SECOND SOCKET, RULED 2026-09-11, and section 14's caller table is the
// reason. stdio is one process per agent and rigd is a singleton, so it is out
// on mechanism; folding MCP into the HTTP surface would merge two rows the
// table exists to keep apart, because an HTTP client gets "its bearer
// principal's scopes, never introspect" while MCP connecting as an
// unregistered socket client gets everything. A second socket is the only
// shape that keeps MCP a caller row of its own.
//
// ONE SERVER PER CONNECTION, and that is the point rather than an
// implementation detail. The principal is minted at accept, exactly as it is
// on the main socket, so a server never holds a principal that could be stale
// for the caller in front of it - which is the shape section 13a's ruling
// requires of anything on a path to invocation.
//
// It mirrors Serve deliberately: same accept loop, same tracking so shutdown
// closes live connections, same reading of a closed listener as "asked to
// stop" rather than as a failure.
func (d *Daemon) ServeMCP(ctx context.Context, l net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()

	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		nc, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil // asked to stop; not a failure
			}
			return fmt.Errorf("daemon: mcp accept: %w", err)
		}
		if !d.track(nc) {
			_ = nc.Close()
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer d.untrack(nc)
			d.serveMCPConn(ctx, nc)
		}()
	}
}

// serveMCPConn runs one agent's MCP session.
func (d *Daemon) serveMCPConn(ctx context.Context, nc net.Conn) {
	defer func() { _ = nc.Close() }()

	// Minted here, per connection, exactly as the main socket mints it.
	//
	// IDENTICAL ON PURPOSE, AND IT IS AN OPEN QUESTION RATHER THAN A
	// CONCLUSION. A connection arriving here is an agent by construction -
	// that is what this socket is for - so there is an argument for minting
	// KindAgent instead of the KindTerminal every unregistered connection
	// gets. It is NOT cosmetic: section 13a's rules table matches on caller
	// kind, so the choice decides whether a house rule written against
	// `agent` fires for an MCP caller at all. Changing it here would be
	// writing that rule in code, which is the thing a worker seat must not
	// do, so it stays identical until it is ruled.
	who := newPrincipal(nc)

	server := mcpserver.New(meta.New(d.kernel, d), who, d.version)
	d.addMCP(server)
	defer d.removeMCP(server)

	// The promoted tools are built before the first request is answered, so
	// an agent's very first tools/list already carries them.
	if err := server.Sync(ctx); err != nil {
		d.log.Warn("could not build the promoted tools", "err", err)
	}

	if err := server.Run(ctx, &mcp.IOTransport{Reader: nc, Writer: nc}); err != nil &&
		ctx.Err() == nil {
		d.log.Debug("mcp session ended", "err", err)
	}
}

// addMCP and removeMCP keep the set of live MCP servers.
func (d *Daemon) addMCP(s *mcpserver.Server) {
	d.mmu.Lock()
	defer d.mmu.Unlock()
	if d.mcps == nil {
		d.mcps = map[*mcpserver.Server]struct{}{}
	}
	d.mcps[s] = struct{}{}
}

func (d *Daemon) removeMCP(s *mcpserver.Server) {
	d.mmu.Lock()
	defer d.mmu.Unlock()
	delete(d.mcps, s)
}

// resyncMCP brings every live MCP server's promoted tools back into line with
// the estate, and is called wherever the registry changes.
//
// THIS IS WHAT MAKES PROMOTION LIVE. Without it a promoted tool list is a
// snapshot of the instant an agent connected: a program that registers
// afterwards is invisible until the agent reconnects, and one that
// disconnects leaves a tool resolving to nothing - which surfaces as a call
// refused for a reason that has nothing to do with the cause.
//
// It is deliberately best-effort and logged rather than returned. A
// registration must not fail because a promoted tool list could not be
// rebuilt; the registry is the truth and the tool list is a projection of it.
func (d *Daemon) resyncMCP(ctx context.Context) {
	d.mmu.Lock()
	servers := make([]*mcpserver.Server, 0, len(d.mcps))
	for s := range d.mcps {
		servers = append(servers, s)
	}
	d.mmu.Unlock()

	for _, s := range servers {
		if err := s.Sync(ctx); err != nil {
			d.log.Warn("could not resync the promoted tools", "err", err)
		}
	}
}
