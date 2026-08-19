# PyPI Compatibility Fixtures

Containerized PyPI client tests for Harbor hosted PyPI support.

Build:

```bash
docker build -f tests/pypi/Containerfile -t harbor-pypi-compat:py312 tests/pypi
```

Run against Harbor on the host:

```bash
docker run --rm \
  --add-host=host.docker.internal:host-gateway \
  -e HARBOR_PYPI_URL=http://host.docker.internal:8080/pypi/library/ \
  -e HARBOR_USERNAME=admin \
  -e HARBOR_PASSWORD=Harbor12345 \
  harbor-pypi-compat:py312
```

Suites:

- `publish`: build a wheel and sdist, then upload with `twine`
- `pip`: install the uploaded package with `pip`
- `uv`: install the uploaded package with `uv`
- `all`: publish, then install with both clients
- `shell`: open an interactive shell

The package version defaults to a timestamp so repeated runs do not collide.
