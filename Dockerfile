# Stage 1 — build frontend assets
FROM node:22-alpine AS frontend
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY web/ web/
COPY internal/views/ internal/views/
RUN npm run build

# Stage 2 — build Go binary
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist web/dist
RUN go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate
RUN go build -o /testrr ./cmd/testrr

# Stage 3 — minimal runtime image
FROM alpine:3.21
RUN adduser -D -u 1000 testrr
USER testrr
WORKDIR /home/testrr
COPY --from=builder /testrr /usr/local/bin/testrr
EXPOSE 8080
ENTRYPOINT ["testrr", "serve"]
