# BitGN PAC1 Go Sample

Runnable Go port of `pac1-py`. Same control-plane flow
(`HarnessService`), same PCM runtime tool surface (`bitgn.vm.pcm`), same SGR
`NextStep` schema, same shell-like tool output shape, same env var knobs.

## Quick start

1. Copy `.env.example` to `.env` and drop in your personal hackathon key:

   ```bash
   cp .env.example .env
   # edit .env and set PROXY_API_KEY=<your-personal-key>
   ```

   The agent auto-loads `./.env` at startup (existing env vars win). A full
   run of the sample can cost ~$2 of the $40 proxy credit, so keep an eye on
   usage.

2. Optional overrides (env vars or extra lines in `.env`):
   - `OPENAI_BASE_URL` (default `http://hackathon-proxy.westeurope.azurecontainer.io:3000/v1`)
   - `PROXY_API_KEY`   — personal hackathon proxy key (do **not** share)
   - `OPENAI_API_KEY`  — only needed if hitting OpenAI directly instead of the proxy
   - `BITGN_HOST`      (default `https://api.bitgn.com`)
   - `BITGN_API_KEY`   — required for leaderboard `StartRun`
   - `BENCH_ID`        (default `bitgn/pac1-dev`)
   - `MODEL_ID`        (default `gpt-4.1-2025-04-14`)
   - `HINT`            — appended to the system prompt

3. Build and run:

   ```bash
   make tidy
   make run
   ```

4. To run a subset of tasks:

   ```bash
   make task TASKS="t01 t03"
   # or
   go run . t01 t03
   ```

## Where does the SDK come from?

The canonical schema lives at <https://buf.build/bitgn/api>, and its source
is checked in to `../proto`. Normally you would just `go get
buf.build/gen/go/bitgn/api/...` and consume the buf.build auto-generated Go
module directly, the same way `pac1-py` consumes
`bitgn-api-connectrpc-python`. At the time of writing the auto-generated Go
package for the `vm` directory does **not** compile on its own — `pcm.proto`
and `mini.proto` both land in the single `bitgn/vm` Go package and their
message types (`ReadRequest`, `WriteRequest`, ...) collide.

To sidestep that, this sample regenerates Go + Connect stubs locally from
`../proto` into `./gen/`, pinning each `.proto` file to its own Go import
path via `Mfile.proto=import/path;package` directives in `buf.gen.yaml`. The
result is committed so `make tidy && make run` works from a clean checkout.

If you have `buf`, `protoc-gen-go`, and `protoc-gen-connect-go` on your
`$PATH` (the flake.nix shell already provides them, and
`go install github.com/bufbuild/buf/cmd/buf@latest`,
`go install google.golang.org/protobuf/cmd/protoc-gen-go@latest`,
`go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest`
work outside it), you can regenerate with:

```bash
make gen
```

## Layout

```
pac1-go/
├── go.mod
├── buf.gen.yaml   # local codegen config for ../proto → ./gen
├── Makefile
├── README.md
├── main.go        # control plane: Status → GetBenchmark → StartRun → StartTrial → EndTrial → SubmitRun
├── agent.go       # 30-step SGR loop + dispatch to PcmRuntime RPCs
├── schema.go      # hand-written NextStep JSON schema + command structs
├── format.go      # shell-like formatters for tree / ls / cat / rg output
└── gen/           # committed, regenerate with `make gen`
    └── bitgn/
        ├── harness/{harness.pb.go, harnessconnect/harness.connect.go}
        └── vm/
            ├── pcm/{pcm.pb.go, pcmconnect/pcm.connect.go}
            └── mini/{mini.pb.go, miniconnect/mini.connect.go}
```
