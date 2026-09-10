// Command fakeapp is the reference program.
//
// PLAN.md section 19: "fakeapp is the misbehaving reference program and ships
// in the repo. It hangs, crashes, leaks, floods, lies about its schema, ignores
// cancellation and returns garbage, each on a flag."
//
// At M0 it behaves, and answers ping. The misbehaviours arrive with the
// supervisor that is supposed to catch them (M6) - a flag that nothing asserts
// against is a flag that rots.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/client"
	"github.com/boris-milner/rig/internal/paths"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fakeapp: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	name := flag.String("name", "fakeapp", "the program id to announce")
	misbehave := flag.String("misbehave", "", "hang (M0 implements only this one)")
	flag.Parse()

	sock, err := paths.Socket()
	if err != nil {
		return err
	}
	c, err := client.Dial(sock)
	if err != nil {
		return err
	}
	defer c.Close()

	c.Handle(func(method string, payload []byte) (proto.Message, error) {
		switch {
		case *misbehave == "hang":
			// Longer than the daemon's CallTimeout, so rig answers DEADLINE
			// instead of waiting on this process.
			time.Sleep(time.Hour)
			return nil, nil
		}
		var req rigv1.PingRequest
		if err := proto.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		return &rigv1.PingResponse{
			Nonce:   req.GetNonce(),
			Program: *name,
			Version: version,
		}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := c.Hello(ctx, *name, version)
	if err != nil {
		return err
	}
	fmt.Printf("%s up: wire %s, rigd %s, scoped %v\n",
		*name, resp.GetWire(), resp.GetDaemonVersion(), resp.GetScoped())

	<-c.Done()
	return c.Err()
}
