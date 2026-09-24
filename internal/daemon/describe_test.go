package daemon

import (
	"errors"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// ⛔ rig.describe ANSWERS THE MCP describe TOOL'S OBJECT, BYTE FOR BYTE.
// Section 10 binds `rig describe --json` to exactly what the MCP tool
// returns, and B10 puts the renderer in the daemon. So the assertion is
// equality with the other door's answer, not a list of fields: a field the
// renderer gains reaches both or the bytes differ.
func TestDescribeOnTheWireIsTheMCPToolsObject(t *testing.T) {
	sock, d := upDaemon(t, nil)
	indexing(t, sock) // registers "shelf"
	ctx := ctx5(t)

	session := dialMCP(ctx, t, upMCP(t, d))
	c := dial(t, sock)

	for _, tc := range []struct{ name, program, command string }{
		{"the program", "shelf", ""},
		{"one command", "shelf", "search"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var resp verbsv1.DescribeResponse
			if err := c.Call(ctx, "rig.describe", &verbsv1.DescribeRequest{
				Program: tc.program, Command: tc.command,
			}, &resp); err != nil {
				t.Fatalf("rig.describe: %v", err)
			}

			args := map[string]any{"program": tc.program}
			if tc.command != "" {
				args["command"] = tc.command
			}
			res, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "describe", Arguments: args})
			if err != nil {
				t.Fatalf("the MCP describe tool: %v", err)
			}
			if res.IsError || len(res.Content) == 0 {
				t.Fatalf("the MCP describe tool refused: %+v", res.Content)
			}
			mcpText := res.Content[0].(*sdk.TextContent).Text

			if string(resp.GetAnswer()) != mcpText {
				t.Fatalf("rig.describe and the MCP describe tool disagree.\n"+
					"wire %s\nmcp  %s", resp.GetAnswer(), mcpText)
			}
		})
	}
}

// An address the caller cannot see is refused on the wire with the MCP tool's
// sentence and a code, never answered with an empty object.
func TestDescribeOnTheWireRefusesWhatItCannotSee(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	indexing(t, sock)
	ctx := ctx5(t)
	c := dial(t, sock)

	for _, tc := range []struct {
		name string
		req  *verbsv1.DescribeRequest
		code rigv1.Code
	}{
		{"no program", &verbsv1.DescribeRequest{}, rigv1.Code_CODE_INVALID},
		{"unknown program", &verbsv1.DescribeRequest{Program: "nope"}, rigv1.Code_CODE_NOT_FOUND},
		{"unknown command", &verbsv1.DescribeRequest{Program: "shelf", Command: "nope"}, rigv1.Code_CODE_NOT_FOUND},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := c.Call(ctx, "rig.describe", tc.req, &verbsv1.DescribeResponse{})
			var re *client.CallError
			if !errors.As(err, &re) {
				t.Fatalf("rig.describe(%v) gave %v, want a refusal", tc.req, err)
			}
			if re.Status.GetCode() != tc.code {
				t.Errorf("refused with %s, want %s: %s", re.Status.GetCode(), tc.code, re.Status.GetMessage())
			}
		})
	}
}
