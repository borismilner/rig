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

// `rig message` - section 16's directed messages at the prompt.
//
//	rig message send <seat> <subject...> [--body B] [--generation G --epoch E]
//	rig message list [seat]
//
// ⛔ ONLY THE TWO VERBS A TERMINAL CAN MEAN. A terminal is named by the daemon
// so it can send, but it holds no seat, and inbox, await and ack belong to the
// seat the message was sent to: the daemon refuses them for an unseated
// connection. Offering them here would be three commands that always fail.

type messageFlags struct {
	fs         *flag.FlagSet
	asJSON     *bool
	timeout    *time.Duration
	body       *string
	generation *uint64
	epoch      *uint64
}

func messageFlagSet() *messageFlags {
	m := &messageFlags{fs: flag.NewFlagSet("message", flag.ContinueOnError)}
	m.asJSON = m.fs.Bool("json", false, "emit JSON")
	m.timeout = m.fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	m.body = m.fs.String("body", "", "send: the message itself")
	m.generation = m.fs.Uint64("generation", 0, "send: pin the seat's generation, from `rig peers` (with --epoch)")
	m.epoch = m.fs.Uint64("epoch", 0, "send: pin the seat's epoch, from `rig peers` (with --generation)")
	return m
}

func cmdMessage(args []string) (err error) {
	m := messageFlagSet()
	flags, positional := partition(args)
	if err := m.fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *m.asJSON) }()
	usage := "usage: rig message send <seat> <subject...> [--body B] " +
		"[--generation G --epoch E] | list [seat]"
	if len(positional) == 0 {
		return badArgumentf("%s", usage)
	}
	sub, rest := positional[0], positional[1:]
	switch {
	case sub == "send" && len(rest) >= 2:
	case sub == "list" && len(rest) <= 1:
	default:
		return badArgumentf("%s", usage)
	}

	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *m.timeout)
	defer cancel()
	out := json.NewEncoder(os.Stdout)

	if sub == "send" {
		var resp verbsv1.MessageSendResponse
		if err := call(ctx, c, "rig.message.send", &verbsv1.MessageSendRequest{
			To: rest[0], Subject: strings.Join(rest[1:], " "), Body: *m.body,
			ToGeneration: *m.generation, ToEpoch: *m.epoch,
		}, &resp); err != nil {
			return err
		}
		if *m.asJSON {
			return out.Encode(map[string]any{
				"message":   messageJSON(resp.GetMessage()),
				"to_state":  seatStateWord(resp.GetToState()),
				"delivered": resp.GetDelivered(),
			})
		}
		msg := resp.GetMessage()
		switch {
		case msg.GetHeldForSuccessor():
			fmt.Printf("#%d held for %s's successor: the seat is handing off\n", msg.GetId(), msg.GetTo())
		case resp.GetDelivered() > 0:
			fmt.Printf("#%d delivered to %s, who was waiting\n", msg.GetId(), msg.GetTo())
		default:
			fmt.Printf("#%d queued for %s; it reads it on its next inbox\n", msg.GetId(), msg.GetTo())
		}
		return nil
	}

	req := &verbsv1.MessageListRequest{}
	if len(rest) == 1 {
		req.Seat = rest[0]
	}
	var resp verbsv1.MessageListResponse
	if err := call(ctx, c, "rig.message.list", req, &resp); err != nil {
		return err
	}
	if *m.asJSON {
		all := make([]map[string]any, 0, len(resp.GetMessages()))
		for _, msg := range resp.GetMessages() {
			all = append(all, messageJSON(msg))
		}
		return out.Encode(map[string]any{"messages": all})
	}
	if len(resp.GetMessages()) == 0 {
		fmt.Println("no messages.")
		return nil
	}
	for _, msg := range resp.GetMessages() {
		fmt.Println(messageLine(msg))
	}
	return nil
}

// messageLine is one message as a person scans it: who to whom, where it got
// to, and the warning that matters most, first.
func messageLine(m *verbsv1.Message) string {
	var b strings.Builder
	fmt.Fprintf(&b, "#%d  %s -> %s  %s  %s", m.GetId(), m.GetFrom(), m.GetTo(),
		messageStateWord(m.GetState()), m.GetSubject())
	if m.GetMisaddressed() {
		fmt.Fprintf(&b, "\n    MISADDRESSED: pinned to generation %d, read by generation %d",
			m.GetToGeneration(), m.GetReadGeneration())
	}
	if m.GetHeldForSuccessor() && m.GetReadGeneration() == 0 {
		b.WriteString("\n    held for the successor")
	}
	if m.GetOutcome() != "" {
		fmt.Fprintf(&b, "\n    outcome: %s", m.GetOutcome())
	}
	return b.String()
}

func messageJSON(m *verbsv1.Message) map[string]any {
	return map[string]any{
		"id": m.GetId(), "to": m.GetTo(), "to_generation": m.GetToGeneration(),
		"to_epoch": m.GetToEpoch(), "from": m.GetFrom(),
		"from_generation": m.GetFromGeneration(), "from_epoch": m.GetFromEpoch(),
		"subject": m.GetSubject(), "body": m.GetBody(),
		"state":              messageStateWord(m.GetState()),
		"held_for_successor": m.GetHeldForSuccessor(),
		"read_generation":    m.GetReadGeneration(), "read_epoch": m.GetReadEpoch(),
		"outcome": m.GetOutcome(), "sent_unix_nano": m.GetSentUnixNano(),
		"moved_unix_nano": m.GetMovedUnixNano(), "misaddressed": m.GetMisaddressed(),
	}
}

// messageStateWord and seatStateWord walk the wire's enum descriptor, as
// stateLabel does, and render a state this build does not know as a number
// rather than as the zero's spelling.
func messageStateWord(s verbsv1.MessageState) string {
	if w, ok := enumWord(s, "MESSAGE_STATE_"); ok {
		return w
	}
	return fmt.Sprintf("state-%d", s)
}

func seatStateWord(s verbsv1.SeatState) string {
	if w, ok := stateLabel(s); ok {
		return w
	}
	return fmt.Sprintf("state-%d", s)
}
