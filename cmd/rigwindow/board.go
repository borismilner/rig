package main

// The board, as the window reads it (plan/55 requirements 1 to 3, 27):
// rig.panel.list for the cards, rig.panel.act for a button, and a wait on
// panel.changed so the page reads again the moment a card moves.

import (
	"context"
	"errors"
	"time"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

const boardEvent = "rig:board"

// Card is one card on the board, as rig.panel.list answers it.
type Card struct {
	ID       string `json:"id"`
	Version  uint64 `json:"version"`
	From     string `json:"from"`
	Project  string `json:"project"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Severity string `json:"severity"`
	Body     string `json:"body"`
	// Progress is 0 to 1; HasProgress says whether there is any.
	Progress    float64  `json:"progress"`
	HasProgress bool     `json:"hasProgress"`
	Busy        bool     `json:"busy"`
	Facts       []Fact   `json:"facts"`
	Actions     []string `json:"actions"`
	Closed      bool     `json:"closed"`
	Created     string   `json:"created"`
	Updated     string   `json:"updated"`
}

// Fact is one label and value on a card.
type Fact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// CardList is the board: every open card and those closed in the last day,
// newest change first, and how many rig left out past its cap.
type CardList struct {
	Cards   []Card `json:"cards"`
	Omitted uint32 `json:"omitted"`
	// Missing is set when the daemon has no board yet, an older rigd.
	Missing bool `json:"missing"`
}

// Board reads the board.
func (RigService) Board() (CardList, error) {
	c, err := client.Connect()
	if err != nil {
		return CardList{}, err
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()
	var resp verbsv1.PanelListResponse
	err = c.Call(ctx, "rig.panel.list", &verbsv1.PanelListRequest{}, &resp)
	var ce *client.CallError
	if errors.As(err, &ce) && ce.Code() == rigv1.Code_CODE_NOT_FOUND {
		return CardList{Cards: []Card{}, Missing: true}, nil
	}
	if err != nil {
		return CardList{}, err
	}
	list := CardList{Cards: make([]Card, 0, len(resp.GetCards())), Omitted: resp.GetOmitted()}
	for _, x := range resp.GetCards() {
		list.Cards = append(list.Cards, cardOf(x))
	}
	return list, nil
}

func cardOf(x *verbsv1.PanelCard) Card {
	c := Card{
		ID: x.GetId(), Version: x.GetVersion(), From: x.GetFrom(), Project: x.GetProject(),
		Title: x.GetTitle(), Status: x.GetStatus(), Severity: x.GetSeverity(), Body: x.GetBody(),
		Progress: x.GetProgress(), HasProgress: x.GetHasProgress(), Busy: x.GetBusy(),
		Facts: []Fact{}, Actions: append([]string{}, x.GetActions()...), Closed: x.GetClosed(),
		Created: stampOf(x.GetCreatedUnixNano()), Updated: stampOf(x.GetUpdatedUnixNano()),
	}
	for _, f := range x.GetFacts() {
		c.Facts = append(c.Facts, Fact{Label: f.GetLabel(), Value: f.GetValue()})
	}
	return c
}

// Press sends one of a card's buttons to its owner. The presser is this
// window's connection, which rig names.
func (RigService) Press(card, action string) error {
	if card == "" || action == "" {
		return errors.New("no card or no action named")
	}
	c, err := client.Connect()
	if err != nil {
		return err
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()
	return c.Call(ctx, "rig.panel.act", &verbsv1.PanelActRequest{Card: card, Action: action}, &verbsv1.PanelActResponse{})
}

// watchBoard waits on panel.changed and tells the page to read the board
// again. A gap, a new epoch or a lost daemon tells it too: the read is the
// truth, and the event is only the reason to look.
func watchBoard(emit func()) {
	var after, epoch uint64
	lost := false
	for {
		resp, err := boardWait(after, epoch)
		if err != nil {
			// Once per loss: an older rigd refuses the kind on every call.
			if !lost {
				emit()
			}
			lost, after, epoch = true, 0, 0
			time.Sleep(trayRefresh)
			continue
		}
		lost = false
		if len(resp.GetEvents()) > 0 || resp.GetGap() || resp.GetEpoch() != epoch {
			emit()
		}
		after, epoch = resp.GetLatest(), resp.GetEpoch()
	}
}

func boardWait(after, epoch uint64) (*registryv1.EventsWaitResponse, error) {
	c, err := client.Connect()
	if err != nil {
		return nil, err
	}
	defer c.Close()
	const park = 50 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), park+readDeadline)
	defer cancel()
	var resp registryv1.EventsWaitResponse
	err = c.Call(ctx, "rig.events.wait", &registryv1.EventsWaitRequest{
		After: after, Epoch: epoch, Kinds: []string{"panel.changed"}, TimeoutMs: uint32(park.Milliseconds()), //nolint:gosec // a 50 s constant
	}, &resp)
	return &resp, err
}
