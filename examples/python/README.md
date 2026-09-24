# greeter.py

A complete rig program in Python, speaking the wire directly. The rules it
follows are in `docs/programs.md` under "Any other language".

Generate the messages from rig's proto files, from the repository root:

    pip install 'protobuf>=6'
    protoc -I . --python_out=examples/python proto/rig/v1/wire.proto

Then, with `rigd` running and `XDG_RUNTIME_DIR` pointing at its runtime
directory:

    python3 examples/python/greeter.py &
    rig pygreeter greet --args '{"name":"Boris"}'

`wire.proto` is all a program needs. The generated `proto/` directory is
ignored by git.
