# A deliberately small, two-layer OCI image for Issue #478 benchmarks.
# The final image contains only a static Go binary and an 8 MiB opaque payload.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY main.go .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /benchmark main.go
RUN head -c 8388608 /dev/urandom > /payload.bin

FROM scratch
COPY --from=build /benchmark /benchmark
COPY --from=build /payload.bin /payload.bin
ENTRYPOINT ["/benchmark"]
