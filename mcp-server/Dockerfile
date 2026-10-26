FROM golang:1.23-alpine AS build

WORKDIR /src
COPY mcp-server/go.mod ./
COPY mcp-server/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/campusclaw-mcp .

FROM alpine:3.21

RUN addgroup -S -g 10002 mcp \
    && adduser -S -D -H -u 10002 -G mcp mcp

WORKDIR /app
COPY --from=build /out/campusclaw-mcp /app/campusclaw-mcp

USER mcp
EXPOSE 8090

ENTRYPOINT ["/app/campusclaw-mcp"]
