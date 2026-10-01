package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/borismilner/rig/internal/kernel"
	"github.com/borismilner/rig/internal/meta"
	"github.com/borismilner/rig/internal/wire"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// plan/09, "The MCP door covers everything rig does": every verb rig declares
// is an MCP tool, and there is no exclusion list (Boris, 2026-09-26).
//
// ⛔ ONE ROUTE FOR ALL OF THEM, AND IT IS THE DAEMON'S OWN DISPATCH. A verb
// called here is framed exactly as the socket would frame it and handed to
// serveSelf on a connection that carries this MCP session's principal and
// seat. So the rules table, the effects ladder, the seat a lease or a claim
// is held under, and every refusal are the ones a terminal meets, from the
// one place that decides them. A second, hand-written door per verb would be
// a second place for each of those to be true or false.

// bridged is every verb the agent door carries through that route, with the
// wire's request and response messages. The tool is the verb with the dot
// replaced, as the record tools are named.
var bridged = map[string]struct {
	req, resp func() proto.Message
}{
	"ping":           {func() proto.Message { return &rigv1.PingRequest{} }, func() proto.Message { return &rigv1.PingResponse{} }},
	"session":        {func() proto.Message { return &verbsv1.SessionRequest{} }, func() proto.Message { return &verbsv1.SessionResponse{} }},
	"lease.list":     {func() proto.Message { return &verbsv1.LeaseListRequest{} }, func() proto.Message { return &verbsv1.LeaseListResponse{} }},
	"lease.acquire":  {func() proto.Message { return &verbsv1.LeaseAcquireRequest{} }, func() proto.Message { return &verbsv1.LeaseAcquireResponse{} }},
	"lease.renew":    {func() proto.Message { return &verbsv1.LeaseRenewRequest{} }, func() proto.Message { return &verbsv1.LeaseRenewResponse{} }},
	"lease.release":  {func() proto.Message { return &verbsv1.LeaseReleaseRequest{} }, func() proto.Message { return &verbsv1.LeaseReleaseResponse{} }},
	"lease.break":    {func() proto.Message { return &verbsv1.LeaseBreakRequest{} }, func() proto.Message { return &verbsv1.LeaseBreakResponse{} }},
	"lease.check":    {func() proto.Message { return &verbsv1.LeaseCheckRequest{} }, func() proto.Message { return &verbsv1.LeaseCheckResponse{} }},
	"queue.push":     {func() proto.Message { return &verbsv1.QueuePushRequest{} }, func() proto.Message { return &verbsv1.QueuePushResponse{} }},
	"queue.claim":    {func() proto.Message { return &verbsv1.QueueClaimRequest{} }, func() proto.Message { return &verbsv1.QueueClaimResponse{} }},
	"queue.complete": {func() proto.Message { return &verbsv1.QueueCompleteRequest{} }, func() proto.Message { return &verbsv1.QueueCompleteResponse{} }},
	"queue.list":     {func() proto.Message { return &verbsv1.QueueListRequest{} }, func() proto.Message { return &verbsv1.QueueListResponse{} }},
	"notify":         {func() proto.Message { return &registryv1.NotifyRequest{} }, func() proto.Message { return &registryv1.NotifyResponse{} }},
	"toast.wait":     {func() proto.Message { return &registryv1.ToastWaitRequest{} }, func() proto.Message { return &registryv1.ToastWaitResponse{} }},
	"toast.dnd":      {func() proto.Message { return &registryv1.ToastDndRequest{} }, func() proto.Message { return &registryv1.ToastDndResponse{} }},
	"toast.reply":    {func() proto.Message { return &registryv1.ToastReplyRequest{} }, func() proto.Message { return &registryv1.ToastReplyResponse{} }},
	"sound":          {func() proto.Message { return &registryv1.SoundRequest{} }, func() proto.Message { return &registryv1.SoundResponse{} }},
	"say":            {func() proto.Message { return &registryv1.SayRequest{} }, func() proto.Message { return &registryv1.SayResponse{} }},
	"toast.answer":   {func() proto.Message { return &registryv1.ToastAnswerRequest{} }, func() proto.Message { return &registryv1.ToastAnswerResponse{} }},
	"up":             {func() proto.Message { return &verbsv1.UpRequest{} }, func() proto.Message { return &verbsv1.UpResponse{} }},
	"stop":           {func() proto.Message { return &verbsv1.StopRequest{} }, func() proto.Message { return &verbsv1.StopResponse{} }},
	verbRestart:      {func() proto.Message { return &verbsv1.RestartRequest{} }, func() proto.Message { return &verbsv1.RestartResponse{} }},
	"health":         {func() proto.Message { return &verbsv1.HealthRequest{} }, func() proto.Message { return &verbsv1.HealthResponse{} }},
	"backup.create":  {func() proto.Message { return &verbsv1.BackupCreateRequest{} }, func() proto.Message { return &verbsv1.BackupCreateResponse{} }},
	"health.report":  {func() proto.Message { return &rigv1.HealthReportRequest{} }, func() proto.Message { return &rigv1.HealthReportResponse{} }},
	// The HANDS OFF strip. hand.answer and hand.strip are carried and refused
	// to an agent (H5, H6): the tool exists so the refusal is the answer, not
	// a missing tool.
	"events.publish":    {func() proto.Message { return &registryv1.EventsPublishRequest{} }, func() proto.Message { return &registryv1.EventsPublishResponse{} }},
	"events.wait":       {func() proto.Message { return &registryv1.EventsWaitRequest{} }, func() proto.Message { return &registryv1.EventsWaitResponse{} }},
	"hand.request":      {func() proto.Message { return &registryv1.HandRequestRequest{} }, func() proto.Message { return &registryv1.HandRequestResponse{} }},
	"hand.step":         {func() proto.Message { return &registryv1.HandStepRequest{} }, func() proto.Message { return &registryv1.HandStepResponse{} }},
	"hand.release":      {func() proto.Message { return &registryv1.HandReleaseRequest{} }, func() proto.Message { return &registryv1.HandReleaseResponse{} }},
	"hand.wait":         {func() proto.Message { return &registryv1.HandWaitRequest{} }, func() proto.Message { return &registryv1.HandWaitResponse{} }},
	"hand.answer":       {func() proto.Message { return &registryv1.HandAnswerRequest{} }, func() proto.Message { return &registryv1.HandAnswerResponse{} }},
	"hand.strip":        {func() proto.Message { return &registryv1.HandStripRequest{} }, func() proto.Message { return &registryv1.HandStripResponse{} }},
	"project.brief":     {func() proto.Message { return &verbsv1.ProjectBriefRequest{} }, func() proto.Message { return &verbsv1.ProjectBriefResponse{} }},
	"down":              {func() proto.Message { return &verbsv1.DownRequest{} }, func() proto.Message { return &verbsv1.DownResponse{} }},
	"files.root":        {func() proto.Message { return &verbsv1.FilesRootRequest{} }, func() proto.Message { return &verbsv1.FilesRootResponse{} }},
	"files.index":       {func() proto.Message { return &verbsv1.FilesIndexRequest{} }, func() proto.Message { return &verbsv1.FilesIndexResponse{} }},
	"files.search":      {func() proto.Message { return &verbsv1.FilesSearchRequest{} }, func() proto.Message { return &verbsv1.FilesSearchResponse{} }},
	"files.unindexed":   {func() proto.Message { return &verbsv1.FilesUnindexedRequest{} }, func() proto.Message { return &verbsv1.FilesUnindexedResponse{} }},
	"files.place":       {func() proto.Message { return &verbsv1.FilesPlaceRequest{} }, func() proto.Message { return &verbsv1.FilesPlaceResponse{} }},
	"files.layout":      {func() proto.Message { return &verbsv1.FilesLayoutRequest{} }, func() proto.Message { return &verbsv1.FilesLayoutResponse{} }},
	"files.relayout":    {func() proto.Message { return &verbsv1.FilesRelayoutRequest{} }, func() proto.Message { return &verbsv1.FilesRelayoutResponse{} }},
	"store.put":         {func() proto.Message { return &verbsv1.StorePutRequest{} }, func() proto.Message { return &verbsv1.StorePutResponse{} }},
	"store.get":         {func() proto.Message { return &verbsv1.StoreGetRequest{} }, func() proto.Message { return &verbsv1.StoreGetResponse{} }},
	"store.query":       {func() proto.Message { return &verbsv1.StoreQueryRequest{} }, func() proto.Message { return &verbsv1.StoreQueryResponse{} }},
	"store.delete":      {func() proto.Message { return &verbsv1.StoreDeleteRequest{} }, func() proto.Message { return &verbsv1.StoreDeleteResponse{} }},
	"store.transact":    {func() proto.Message { return &verbsv1.StoreTransactRequest{} }, func() proto.Message { return &verbsv1.StoreTransactResponse{} }},
	"store.collections": {func() proto.Message { return &verbsv1.StoreCollectionsRequest{} }, func() proto.Message { return &verbsv1.StoreCollectionsResponse{} }},
	"store.list":        {func() proto.Message { return &verbsv1.StoreListRequest{} }, func() proto.Message { return &verbsv1.StoreListResponse{} }},
	"store.export":      {func() proto.Message { return &verbsv1.StoreExportRequest{} }, func() proto.Message { return &verbsv1.StoreExportResponse{} }},
	"store.import":      {func() proto.Message { return &verbsv1.StoreImportRequest{} }, func() proto.Message { return &verbsv1.StoreImportResponse{} }},
}

// servedByName is every verb the agent door already carries under a tool of
// its own, and that tool's name.
var servedByName = map[string]meta.Tool{
	"programs": meta.List, "describe": meta.Describe, "estate": meta.Query,
	"announce": meta.Announce, "activity": meta.SetActivity, "peers": meta.ListAgents,
	"record.put": meta.RecordPutTool, "record.get": meta.RecordGetTool,
	"record.query": meta.RecordQueryTool, "record.history": meta.RecordHistoryTool,
	"record.link": meta.RecordLinkTool, "record.unlink": meta.RecordUnlinkTool,
	"record.refs": meta.RecordRefsTool, "record.retract": meta.RecordRetractTool,
	"record.delete": meta.RecordDeleteTool, "record.replace": meta.RecordReplaceTool,
	"progress.step":    meta.ProgressStepTool,
	"knowledge.search": meta.KnowledgeSearchTool, "knowledge.get": meta.KnowledgeGetTool,
	"knowledge.add":  meta.KnowledgeAddTool,
	"worknote.write": meta.WorkNoteWriteTool, "worknote.mine": meta.WorkNoteMineTool,
	"worknote.about": meta.WorkNoteAboutTool,
	"message.send":   meta.MessageSendTool, verbMessageInbox: meta.MessageInboxTool,
	"message.await": meta.MessageAwaitTool, "message.ack": meta.MessageAckTool,
	"message.list": meta.MessageListTool,
}

// verbMessageInbox is named in the dispatch, in message.go and here.
const verbMessageInbox = "message.inbox"

// verbTool is the MCP tool name for a verb.
func verbTool(command string) string { return strings.ReplaceAll(command, ".", "_") }

var _ meta.Verbs = (*mcpCaller)(nil)

// Verbs lists the bridged verbs in the order rig declares them, with the
// declaration's own words and effects.
func (m *mcpCaller) Verbs() []meta.Verb {
	var out []meta.Verb
	for _, c := range selfDeclaration().Commands {
		b, ok := bridged[c.ID]
		if !ok {
			continue
		}
		desc := c.Description
		if c.Returns != "" {
			desc += " Returns: " + c.Returns
		}
		out = append(out, meta.Verb{
			Tool: verbTool(c.ID), Command: c.ID, Title: c.Title,
			Description: desc, Effects: c.Effects, Idempotent: c.Idempotent,
			Input: inputSchema(b.req().ProtoReflect().Descriptor()),
		})
	}
	return out
}

// CallVerb runs one bridged verb through serveSelf, as this connection.
func (m *mcpCaller) CallVerb(ctx context.Context, command string, args []byte) ([]byte, error) {
	b, ok := bridged[command]
	if !ok {
		return nil, fmt.Errorf("rig.%s is not carried on the agent door", command)
	}
	req := b.req()
	// A client that sends no arguments sends null, which is "nothing" and
	// not a malformed object.
	if a := bytes.TrimSpace(args); len(a) > 0 && !bytes.Equal(a, []byte("null")) {
		if err := protojson.Unmarshal(args, req); err != nil {
			return nil, &kernel.RefusalError{
				Err:          fmt.Errorf("%s: the arguments do not fit the verb: %w", verbTool(command), err),
				Precondition: "the arguments match the tool's input schema",
				Actual:       "a field that is unknown or of the wrong type",
				Fix:          "call it again with the fields the schema names",
			}
		}
	}
	payload, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}

	// The connection is in memory and lives for one call. It carries the MCP
	// session's occupancy and principal, which is all provenance and the
	// rules table read, and never registers, so it is never scoped. Found
	// live: without agentDoor an unannounced agent held a lease as
	// "terminal:<user>", because the MCP socket mints terminals.
	var out frameSink
	c := &conn{
		w: wire.NewConn(&out), log: m.log, occ: m.occ, agentDoor: true,
		pending: make(map[uint32]chan *rigv1.Frame),
	}
	c.who.Store(m.who)
	m.serveSelf(ctx, c, &rigv1.Frame{
		StreamId: 1, Kind: rigv1.FrameKind_FRAME_KIND_REQUEST,
		Method: kernel.SelfID + "." + command, Payload: payload,
	}, command)

	f, err := wire.NewConn(&out).ReadFrame()
	if err != nil {
		return nil, fmt.Errorf("rig.%s answered nothing: %w", command, err)
	}
	if st := f.GetStatus(); f.GetKind() == rigv1.FrameKind_FRAME_KIND_ERROR {
		return nil, &kernel.RefusalError{
			Err: errors.New(st.GetMessage()), Precondition: st.GetPrecondition(),
			Actual: st.GetActual(), Fix: st.GetFix(), FixCommand: st.GetFixCommand(),
		}
	}
	resp := b.resp()
	if err := proto.Unmarshal(f.GetPayload(), resp); err != nil {
		return nil, err
	}
	return protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(resp)
}

// frameSink is the in-memory connection's other end: what serveSelf writes is
// read back once it returns.
type frameSink struct{ bytes.Buffer }

func (*frameSink) Close() error { return nil }

// inputSchema is a request message as a JSON Schema, in the field names
// protojson reads. Enums take their value names; 64-bit integers are accepted
// as numbers.
func inputSchema(md protoreflect.MessageDescriptor) json.RawMessage {
	b, err := json.Marshal(messageSchema(md, 0))
	if err != nil {
		return json.RawMessage(`{"type":"object"}`)
	}
	return b
}

func messageSchema(md protoreflect.MessageDescriptor, depth int) map[string]any {
	props := map[string]any{}
	fields := md.Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		s := fieldSchema(fd, depth)
		if fd.IsList() {
			s = typed("array", "items", s)
		}
		props[fd.JSONName()] = s
	}
	return typed("object", "properties", props, "additionalProperties", false)
}

func fieldSchema(fd protoreflect.FieldDescriptor, depth int) map[string]any {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return typed("boolean")
	case protoreflect.StringKind:
		return typed("string")
	case protoreflect.BytesKind:
		return typed("string", "contentEncoding", "base64")
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return typed("number")
	case protoreflect.EnumKind:
		vals := fd.Enum().Values()
		names := make([]string, 0, vals.Len())
		for i := range vals.Len() {
			names = append(names, string(vals.Get(i).Name()))
		}
		return typed("string", "enum", names)
	case protoreflect.MessageKind, protoreflect.GroupKind:
		if depth >= 3 || fd.IsMap() {
			return typed("object")
		}
		return messageSchema(fd.Message(), depth+1)
	default:
		return typed("integer")
	}
}

// typed is a schema node of one JSON type, with further keyword and value
// pairs.
func typed(t string, kv ...any) map[string]any {
	out := map[string]any{"type": t}
	for i := 0; i+1 < len(kv); i += 2 {
		if k, ok := kv[i].(string); ok {
			out[k] = kv[i+1]
		}
	}
	return out
}
