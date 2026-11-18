FROM node:22-bookworm-slim AS web
WORKDIR /web
COPY web/package*.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.25-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
COPY gameengine ./gameengine
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /server ./cmd/server

FROM scratch
WORKDIR /app
COPY --from=backend /server ./server
COPY --from=web /web/dist ./web/dist
COPY scenarios ./scenarios
ENV DB_PATH=/data/game.db
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/app/server"]
