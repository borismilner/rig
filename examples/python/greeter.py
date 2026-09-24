# greeter.py: a complete rig program in Python, from the proto files alone.
#
# The same program as examples/greeter in Go, speaking the wire directly:
# see docs/programs.md, "Any other language". Generate the messages first
# (README.md beside this file), start rigd, then:
#
#   python3 greeter.py &
#   rig pygreeter greet --args '{"name":"Boris"}'
import json, os, socket, struct, sys
sys.path.insert(0, os.path.dirname(__file__))
from proto.rig.v1 import wire_pb2 as w

def send(sock, frame):
    body = frame.SerializeToString()
    sock.sendall(struct.pack(">I", len(body)) + body)

def recv(sock):
    head = sock.recv(4, socket.MSG_WAITALL)
    if len(head) < 4:
        raise EOFError
    (n,) = struct.unpack(">I", head)
    f = w.Frame()
    f.ParseFromString(sock.recv(n, socket.MSG_WAITALL))
    return f

sock = socket.socket(socket.AF_UNIX)
sock.connect(os.path.join(os.environ["XDG_RUNTIME_DIR"], "rig", "rigd.sock"))

cmd = w.Command(id="greet", title="Greet", summary="Say hello", description="Greets a name.",
    returns="A greeting.", effects=w.EFFECTS_READ_ONLY, idempotent=w.TRISTATE_YES,
    sensitive=w.SensitiveFields(), interactive=w.TRISTATE_NO, streams=w.TRISTATE_NO,
    needs_display=w.TRISTATE_NO, duration=w.DURATION_INSTANT, confirms=w.TRISTATE_NO,
    shape=w.SHAPE_UNARY,
    args=json.dumps({"type": "object", "properties": {"name": {"type": "string"}},
                     "required": ["name"]}).encode())
decl = w.Declaration(identity=w.Identity(id="pygreeter", name="Py Greeter", version="0.1",
    description="Says hello, from Python."), coverage=w.COVERAGE_PARTIAL,
    coverage_note="the wire only", semantics_gen=1, commands=[cmd])
hello = w.HelloRequest(program="pygreeter", version="0.1", declaration=decl)
send(sock, w.Frame(stream_id=1, kind=w.FRAME_KIND_REQUEST, method="rig.hello",
                   payload=hello.SerializeToString()))
reply = recv(sock)
if reply.kind != w.FRAME_KIND_RESPONSE:
    sys.exit("hello refused: " + reply.status.message)
print("pygreeter registered", flush=True)

while True:
    f = recv(sock)
    if f.kind != w.FRAME_KIND_REQUEST:
        continue
    command = f.method.split(".", 1)[1]
    if command == "ping":
        req = w.PingRequest(); req.ParseFromString(f.payload)
        out = w.PingResponse(nonce=req.nonce, program="pygreeter", version="0.1")
    elif command == "greet":
        req = w.CallRequest(); req.ParseFromString(f.payload)
        name = json.loads(req.args or b"{}").get("name", "")
        if not name:
            # A refusal the caller can act on: code, precondition, actual, fix.
            send(sock, w.Frame(stream_id=f.stream_id, kind=w.FRAME_KIND_ERROR,
                 status=w.Status(code=w.CODE_INVALID, message="pygreeter: greet needs a name",
                                 precondition="the arguments carry a non-empty name",
                                 actual="no name was given", fix="pass a name")))
            continue
        out = w.CallResponse(result=json.dumps({"greeting": "hello, " + name}).encode())
    else:
        send(sock, w.Frame(stream_id=f.stream_id, kind=w.FRAME_KIND_ERROR,
             status=w.Status(code=w.CODE_NOT_FOUND, message="no command " + command)))
        continue
    send(sock, w.Frame(stream_id=f.stream_id, kind=w.FRAME_KIND_RESPONSE,
                       payload=out.SerializeToString()))
