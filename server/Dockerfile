FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/nover-api ./cmd/api

FROM alpine:3.22
RUN addgroup -S nover && adduser -S -G nover nover && mkdir -p /var/lib/nover/files && chown -R nover:nover /var/lib/nover
COPY --from=build /out/nover-api /usr/local/bin/nover-api
USER nover
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/nover-api"]
