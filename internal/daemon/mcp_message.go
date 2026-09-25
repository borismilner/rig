package daemon

import (
	"context"
	"strings"

	"github.com/borismilner/rig/internal/coord"
	"github.com/borismilner/rig/internal/meta"
)

// The message tools are the other door onto message.go's core. Everything here
// maps; sendMail, readMail, ackMail and listMail decide.
var _ meta.Mailbox = (*mcpCaller)(nil)

// me is this connection's row, or an unseated occupant the core refuses by name.
func (m *mcpCaller) me() occupant {
	o, _ := m.presence.occupantOf(m.occ)
	return o
}

// SendMail sends as this connection's seat. An MCP caller has no terminal name
// to fall back on, so an unannounced one is refused rather than named.
func (m *mcpCaller) SendMail(_ context.Context, in meta.MailRequest) (meta.MailSent, error) {
	o := m.me()
	sent, err := m.sendMail(o.seat, o.generation, o.epoch, mailSend{
		To: in.To, ToGeneration: in.ToGeneration, ToEpoch: in.ToEpoch,
		Subject: in.Subject, Body: in.Body,
	})
	if err != nil {
		return meta.MailSent{}, err
	}
	return meta.MailSent{
		Message:   mailToMeta(sent.Message),
		ToState:   stateWord(sent.ToState),
		Delivered: sent.Delivered,
	}, nil
}

func (m *mcpCaller) ReadMail(ctx context.Context, after uint64, limit int, waitMs uint32, await bool) (meta.MailBatch, error) {
	wait := awaitWait(waitMs)
	if !await {
		wait = 0
	}
	b, err := m.readMail(ctx, m.me(), after, limit, wait)
	if err != nil {
		return meta.MailBatch{}, err
	}
	out := meta.MailBatch{
		Cursor: b.Cursor, Gap: b.Gap, TimedOut: b.TimedOut,
		Misaddressed: b.Misaddressed,
		Messages:     make([]meta.Mail, 0, len(b.Messages)),
	}
	for _, msg := range b.Messages {
		out.Messages = append(out.Messages, mailToMeta(msg))
	}
	return out, nil
}

// AckMail takes the state as the word an agent types, in either case. Anything
// but acknowledged or acted_on reaches ackMail as itself and is refused there.
func (m *mcpCaller) AckMail(_ context.Context, id uint64, state, outcome string) (meta.Mail, error) {
	st := coord.MessageState(strings.ToUpper(strings.ReplaceAll(state, "-", "_")))
	msg, err := m.ackMail(m.me(), id, st, outcome)
	if err != nil {
		return meta.Mail{}, err
	}
	return mailToMeta(msg), nil
}

func (m *mcpCaller) ListMail(_ context.Context, seat string) ([]meta.Mail, error) {
	all, err := m.listMail(seat)
	if err != nil {
		return nil, err
	}
	out := make([]meta.Mail, 0, len(all))
	for _, msg := range all {
		out = append(out, mailToMeta(msg))
	}
	return out, nil
}

// mailToMeta is messageToWire's twin for the agent surface, including the
// after-the-fact misaddressed flag computed the same way.
func mailToMeta(m coord.Message) meta.Mail {
	return meta.Mail{
		ID: m.ID, To: m.To, ToGeneration: m.ToGeneration, ToEpoch: m.ToEpoch,
		From: m.From, FromGeneration: m.FromGeneration, FromEpoch: m.FromEpoch,
		Subject: m.Subject, Body: m.Body,
		State:            strings.ToLower(string(m.State)),
		HeldForSuccessor: m.HeldForSuccessor,
		ReadGeneration:   m.ReadGeneration, ReadEpoch: m.ReadEpoch,
		Outcome:      m.Outcome,
		SentUnixNano: m.SentUnixNano, MovedUnixNano: m.MovedUnixNano,
		Misaddressed: m.ReadGeneration != 0 && m.MisaddressedTo(m.ReadGeneration, m.ReadEpoch),
	}
}
