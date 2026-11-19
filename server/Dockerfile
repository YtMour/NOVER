FROM golang:1.26-alpine AS build
WORKDIR /src

# Release metadata is injected into /api/v1/meta and startup logs.
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILT_AT=unknown
ENV CGO_ENABLED=0 GOOS=linux

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags "-s -w -X github.com/YtMour/NOVER/server/internal/buildinfo.Version=${VERSION} -X github.com/YtMour/NOVER/server/internal/buildinfo.Commit=${COMMIT} -X github.com/YtMour/NOVER/server/internal/buildinfo.BuiltAt=${BUILT_AT}" -o /out/nover-api ./cmd/api
RUN go build -trimpath -ldflags "-s -w -X github.com/YtMour/NOVER/server/internal/buildinfo.Version=${VERSION} -X github.com/YtMour/NOVER/server/internal/buildinfo.Commit=${COMMIT} -X github.com/YtMour/NOVER/server/internal/buildinfo.BuiltAt=${BUILT_AT}" -o /out/nover-maintenance ./cmd/maintenance
RUN go build -trimpath -ldflags "-s -w -X github.com/YtMour/NOVER/server/internal/buildinfo.Version=${VERSION} -X github.com/YtMour/NOVER/server/internal/buildinfo.Commit=${COMMIT} -X github.com/YtMour/NOVER/server/internal/buildinfo.BuiltAt=${BUILT_AT}" -o /out/nover-migrate ./cmd/migrate

FROM alpine:3.22
RUN addgroup -S nover && adduser -S -G nover nover && mkdir -p /var/lib/nover/files && chown -R nover:nover /var/lib/nover
COPY --from=build /out/nover-api /usr/local/bin/nover-api
COPY --from=build /out/nover-maintenance /usr/local/bin/nover-maintenance
COPY --from=build /out/nover-migrate /usr/local/bin/nover-migrate
USER nover
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/nover-api"]
