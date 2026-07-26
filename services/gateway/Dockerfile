FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download

COPY services/gateway ./services/gateway
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/gateway ./services/gateway

FROM scratch

USER 65532:65532
COPY --from=build /out/gateway /gateway

EXPOSE 8080
ENTRYPOINT ["/gateway"]
