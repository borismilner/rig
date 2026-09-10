// Command rig is the front door.
//
// The other of the two binaries (PLAN.md section 22). It never links the
// daemon's internals: it reaches rig over the socket the same way every other
// client does, which is also why the CLI cannot become a second implementation
// of anything (section 5d).
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/boris-milner/rig/internal/client"
	"github.com/boris-milner/rig/internal/paths"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

var (
	version = "dev"
	wire    = "v1"
	sha     = "none"
	date    = "unknown"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "rig: "+err.Error())
		os.Exit(1)
	}
}

// partition splits flags from positionals so a flag may appear anywhere.
//
// Go's flag package stops at the first non-flag argument, so `rig ping fakeapp
// --json` silently treats --json as a positional and the command fails with a
// usage error. Section 10 promises "--json on everything", and a promise that
// depends on argument order is not one. Found by running it.
func partition(args []string) (flags, positional []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return flags, append(positional, args[i+1:]...)
		case strings.HasPrefix(a, "-") && a != "-":
			flags = append(flags, a)
			// A flag written as `--timeout 5s` takes the next argument, while
			// `--timeout=5s` and a boolean do not.
			if !strings.Contains(a, "=") && i+1 < len(args) &&
				!strings.HasPrefix(args[i+1], "-") && takesValue(a) {
				i++
				flags = append(flags, args[i])
			}
		default:
			positional = append(positional, a)
		}
	}
	return flags, positional
}

// takesValue reports whether a flag consumes the following argument. The set is
// tiny and explicit: guessing from the next token's shape is what makes
// `rig ping --timeout 5s fakeapp` and `rig ping --json fakeapp` disagree.
func takesValue(flag string) bool {
	name := strings.TrimLeft(flag, "-")
	switch name {
	case "timeout":
		return true
	}
	return false
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: rig <command> [flags]

  ping <program>   round-trip a program through rigd ("rig" pings the daemon)
  version          print every version this build carries

M0 ships these two. Every other command arrives with the registry at M1.
`)
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("no command given")
	}

	switch args[0] {
	case "version":
		return cmdVersion(args[1:])
	case "ping":
		return cmdPing(args[1:])
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("no such command %q", args[0])
	}
}

// cmdVersion prints all three versions plus the build, because they are not the
// same number and section 28 says so.
func cmdVersion(args []string) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	flags, _ := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{
			"product": version, "wire": wire, "commit": sha, "built": date,
		})
	}
	fmt.Printf("product %s\nwire    %s\ncommit  %s\nbuilt   %s\n", version, wire, sha, date)
	return nil
}

func cmdPing(args []string) error {
	fs := flag.NewFlagSet("ping", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	timeout := fs.Duration("timeout", 5*time.Second, "how long to wait")
	flags, positional := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	if len(positional) != 1 {
		return errors.New("usage: rig ping <program> [--json] [--timeout=5s]")
	}
	program := positional[0]

	sock, err := paths.Socket()
	if err != nil {
		return err
	}
	c, err := client.Dial(sock)
	if err != nil {
		// The one error a user hits constantly, so it says what to do.
		return fmt.Errorf("%w\n       is rigd running? start it with: rigd", err)
	}
	defer c.Close()

	// A fresh nonce per call, echoed back, so the round trip is provably this
	// call's and not a cached or crossed reply.
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	start := time.Now()
	resp := &rigv1.PingResponse{}
	if err := c.Call(ctx, program+".ping", &rigv1.PingRequest{Nonce: nonce}, resp); err != nil {
		return err
	}
	elapsed := time.Since(start)

	if string(resp.GetNonce()) != string(nonce) {
		return fmt.Errorf("%s answered with the wrong nonce: the round trip is not ours", program)
	}

	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"program": resp.GetProgram(),
			"version": resp.GetVersion(),
			"rtt_us":  elapsed.Microseconds(),
		})
	}
	fmt.Printf("%s %s, round trip %s\n", resp.GetProgram(), resp.GetVersion(),
		elapsed.Round(time.Microsecond))
	return nil
}
