FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ENV CGO_ENABLED=0 GOARCH=amd64
RUN LDFLAGS="-s -w -X smtbench/internal/buildinfo.Version=${VERSION}" && \
    GOOS=linux go build -ldflags "$LDFLAGS" -o /out/smtbench-server ./cmd/smtbench-server && \
    GOOS=linux go build -ldflags "$LDFLAGS" -o /out/download/smtbench-linux-amd64 ./cmd/smtbench && \
    GOOS=windows go build -ldflags "$LDFLAGS" -o /out/download/smtbench-windows-amd64.exe ./cmd/smtbench

FROM alpine:3.20
RUN adduser -D -u 10001 app && mkdir -p /data /app/download && chown app /data
COPY --from=build /out/smtbench-server /app/smtbench-server
COPY --from=build /out/download/ /app/download/
USER app
EXPOSE 8080
VOLUME /data
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/smtbench-server"]
