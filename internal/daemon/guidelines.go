package daemon

import (
	"google.golang.org/protobuf/proto"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"

	"github.com/borismilner/rig/internal/guidelines"
	"github.com/borismilner/rig/internal/kernel"
)

// serveGuidelines answers rig.guidelines: the dated rules, and when each
// program the caller may see was built (plan/55 requirement 29, decision
// 0266).
//
// The binary's path comes from the supervisor's spec, never from the
// request, and the file is read for its build info and never run.
func (d *Daemon) serveGuidelines(c *conn, f *rigv1.Frame) {
	var req verbsv1.GuidelinesRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "guidelines: "+err.Error())
		return
	}
	rules, err := guidelines.Rules()
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	estate, err := d.kernel.See(c.principal()).Estate(kernel.DepthPrograms)
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INVALID, err)
		return
	}
	resp := &verbsv1.GuidelinesResponse{Revision: guidelines.Revision(rules)}
	for _, r := range rules {
		resp.Rules = append(resp.Rules, &verbsv1.Guideline{
			Id: r.ID, Date: r.Date, Who: r.Who, Built: r.Built,
			Title: r.Title, Body: r.Body, Cite: r.Cite,
		})
	}
	for _, p := range estate {
		id := p.Identity.ID
		var path string
		// Without supervision no binary is known, and Built says so.
		if d.super != nil {
			if spec, ok := d.super.Spec(id); ok {
				path = spec.Path
			}
		}
		b := guidelines.Built(path)
		resp.Programs = append(resp.Programs, &verbsv1.ProgramBuild{
			Program: id, CommitTime: b.CommitTime, Modified: b.Modified,
			UnknownBecause: b.UnknownBecause,
		})
	}
	c.reply(f.GetStreamId(), resp)
}
