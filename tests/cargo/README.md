# Cargo Compatibility Fixtures

Containerized Cargo client tests for Harbor hosted Cargo support.

Build:

```bash
docker build -f tests/cargo/Containerfile -t harbor-cargo-compat:rust184 tests/cargo
```

Run against Harbor on the host:

```bash
docker run --rm \
  --add-host=host.docker.internal:host-gateway \
  -e HARBOR_CARGO_URL=http://host.docker.internal:8080/cargo/library/ \
  -e HARBOR_USERNAME=admin \
  -e HARBOR_PASSWORD=Harbor12345 \
  harbor-cargo-compat:rust184
```

Suites:

- `publish`: publish a generated binary crate with `cargo publish`
- `install`: install the published crate with `cargo install`
- `dependency`: resolve the published crate as a dependency from a clean project
- `all`: publish, install, and dependency resolution
- `shell`: open an interactive shell

The Cargo token is a base64 encoded `username:password` value because Harbor's
initial Cargo handler accepts that token format for compatibility with Harbor
local credentials.
