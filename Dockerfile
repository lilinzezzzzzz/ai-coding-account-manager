FROM golang:1.26.4-alpine3.24 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/ai-coding-account-manager ./cmd/ai-coding-account-manager

FROM alpine:3.24

RUN adduser -D -H -u 10001 app
WORKDIR /app
COPY --from=build /out/ai-coding-account-manager /app/ai-coding-account-manager
COPY frontend/static /app/frontend/static
RUN mkdir -p /data /codex && chown -R app:app /data /codex /app

USER app
ENV AI_CODING_ACCOUNT_MANAGER_BIND_ADDR=0.0.0.0:43127
ENV AI_CODING_ACCOUNT_MANAGER_CONTAINER=1
ENV AI_CODING_ACCOUNT_MANAGER_DATA_DIR=/data
ENV CODEX_HOME=/codex
EXPOSE 43127

ENTRYPOINT ["/app/ai-coding-account-manager"]
