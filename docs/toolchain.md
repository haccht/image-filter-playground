# Toolchain & Build Notes

## Verified Requirements

| Tool / Dependency | Version |
| --- | --- |
| Go | `>= 1.27.1` (tested with `go1.27.4`) |
| Spin CLI | `4.3.0+` |
| Spin Go SDK | `github.com/spinframework/spin-go-sdk/v4 v4.0.0` |
| `componentize-go` | `v0.5.3` (pinned via `go.mod` `tool` directive) |

`componentize-go` compiles standard Go packages into a **WASIp3** WebAssembly component (`main.wasm`) using its automatically downloaded async-capable Go toolchain bootstrap. TinyGo is not required for the Spin 4 / SDK v4 build.

## Build Configuration (`spin.toml`)

```toml
[component.transform.build]
# Let componentize-go select its async-capable Go without an inherited GOROOT.
command = "env -u GOROOT go tool componentize-go build"
watch = ["**/*.go", "internal/transform/index.html", "go.mod", "go.sum"]
```

### Important Build Notes

1. **Fetch modules before first build**:
   Run `go mod download` before running `spin build` for the first time so that `spin-go-sdk/v4` and its bundled WIT definitions are present in the module cache.
2. **Unsetting `GOROOT`**:
   On systems where `GOROOT` is explicitly exported in the environment (for example, certain Homebrew setups), `componentize-go`'s downloaded async-capable Go compiler may mistakenly read the host's runtime sources and fail with `downloaded Go does not support async`. Running `env -u GOROOT go tool componentize-go build` scopes the fix strictly to the component build step without altering your shell profile.
3. **Runtime target compatibility**:
   The resulting `main.wasm` targets Spin 4.x and **Akamai Functions (`spin aka`)**, which support WASIp3 (`wasi:http@0.3.0`) and Component Model async/backpressure intrinsics. Legacy Spin 2/3 hosts (such as the older `spin cloud` plugin target) do not support WASIp3 components.

## References

- [Spin 4 Go Components Guide](https://spinframework.dev/v4/go-components)
- [Spin Go SDK v4.0.0](https://github.com/spinframework/spin-go-sdk/tree/v4.0.0)
- [componentize-go v0.5.3](https://github.com/bytecodealliance/componentize-go/tree/v0.5.3)
- [Akamai Functions Quickstart](https://techdocs.akamai.com/akamai-functions/docs/quickstart)
