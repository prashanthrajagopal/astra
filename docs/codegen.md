# Code generation

## Protobuf / gRPC

Generated Go code lives under `proto/kernel/` and `proto/tasks/`. To regenerate after editing `.proto` files:

- From repo root: `buf generate`
- Ensure `buf` is on your PATH (e.g. `go install github.com/bufbuild/buf/cmd/buf@latest`).
- Buf uses remote plugins (`buf.build/protocolbuffers/go`, `buf.build/grpc/go`); no local `protoc` or plugins required.

CI does not run `buf generate`; generated `.pb.go` and `_grpc.pb.go` files are committed so `go build ./...` works without buf.
