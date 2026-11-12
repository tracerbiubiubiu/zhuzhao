# Build context: deployments/tools/migrate
FROM golang:1.27.4-alpine3.24@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS builder
ARG GOPROXY=https://proxy.golang.org,direct
ARG TARGETOS
ARG TARGETARCH
ENV GOTOOLCHAIN=local GOWORK=off GOSUMDB=sum.golang.org \
    CGO_ENABLED=0 GOMAXPROCS=2 GOMEMLIMIT=768MiB
WORKDIR /build
COPY go.mod go.sum ./
RUN --mount=type=cache,id=zhuzhao-tool-mod-go1274,target=/go/pkg/mod \
    --mount=type=cache,id=zhuzhao-tool-build-go1274,target=/root/.cache/go-build \
    set -eu; export GOOS="$TARGETOS" GOARCH="$TARGETARCH"; \
    go build -mod=readonly -p=2 -trimpath -buildvcs=false -tags=postgres \
      -ldflags='-X main.Version=4.20.1' -o /out/migrate \
      github.com/golang-migrate/migrate/v4/cmd/migrate \
    && go mod verify \
    && go list -mod=readonly -tags=postgres -deps \
      -f '{{with .Module}}{{.Path}} {{.Version}} {{.Dir}}{{end}}' \
      github.com/golang-migrate/migrate/v4/cmd/migrate > /out/modules-unsorted.txt \
    && sort -u /out/modules-unsorted.txt > /out/modules.txt \
    && mkdir -p /out/licenses \
    && : > /out/licenses/missing-root-license.txt \
    && while read -r module_path module_version module_dir; do \
      [ -n "$module_path" ] || continue; \
      license_dir="/out/licenses/$module_path@$module_version"; \
      mkdir -p "$license_dir" || exit 1; \
      license_found=0; \
      for license_file in "$module_dir"/LICENSE* "$module_dir"/NOTICE* \
                          "$module_dir"/COPYING* "$module_dir"/COPYRIGHT*; do \
        if [ -f "$license_file" ]; then \
          cp "$license_file" "$license_dir/" || exit 1; license_found=1; \
        fi; \
      done; \
      if [ "$license_found" -eq 0 ]; then \
        printf '%s\n' "$module_path@$module_version" >> /out/licenses/missing-root-license.txt || exit 1; \
      fi; \
    done < /out/modules.txt \
    && cp /usr/local/go/LICENSE /out/licenses/Go-LICENSE \
    && test -s /out/licenses/github.com/golang-migrate/migrate/v4@v4.20.1/LICENSE \
    && { printf '%s\n' \
      'Upstream: https://github.com/golang-migrate/migrate/tree/v4.20.1' \
      'Rebuild: Go1.27.4; postgres driver; file source; symbols retained'; \
      go version -m /out/migrate; sha256sum /out/migrate; \
    } > /out/build-info.txt

FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
LABEL org.opencontainers.image.source="https://github.com/golang-migrate/migrate" \
      org.opencontainers.image.version="4.20.1" \
      org.opencontainers.image.description="Zhuzhao rebuild with Go1.27.4 and only postgres/file drivers"
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /out/migrate /usr/local/bin/migrate
COPY --from=builder /out/licenses/ /usr/share/licenses/migrate/
COPY --from=builder /out/build-info.txt /out/modules.txt /usr/share/doc/migrate/
RUN ln -s /usr/local/bin/migrate /migrate
ENTRYPOINT ["migrate"]
CMD ["--help"]
