FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download

COPY services/gateway ./services/gateway
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/gateway ./services/gateway

FROM scratch

ENV HOST=0.0.0.0
USER 65532:65532
COPY --from=build /out/gateway /gateway

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD ["/gateway", "healthcheck"]
ENTRYPOINT ["/gateway"]
