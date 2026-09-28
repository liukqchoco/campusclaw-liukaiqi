FROM golang:1.23-alpine AS build

WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/campusclaw-api ./cmd/server

FROM alpine:3.21

RUN addgroup -S -g 10001 campusclaw \
    && adduser -S -D -H -u 10001 -G campusclaw campusclaw \
    && mkdir -p /app/uploads \
    && chown -R campusclaw:campusclaw /app

WORKDIR /app
COPY --from=build /out/campusclaw-api /app/campusclaw-api

USER campusclaw
EXPOSE 8081

ENTRYPOINT ["/app/campusclaw-api"]
