# Build stage
FROM golang:1.26-alpine AS builder

# libstdc++/libgcc are required by the tailwind v4 standalone binary (Bun runtime)
RUN apk update && apk add --no-cache git wget gcompat libstdc++ libgcc

WORKDIR /src

# Download dependencies first (layer cache)
COPY go.mod go.sum ./
RUN go mod download

# Copy source (mirrors Dagger CI exclusions)
COPY . .

# Download tailwind binary used by htmgo build
RUN mkdir -p __htmgo && \
  wget -q -O __htmgo/tailwind \
  https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-x64 && \
  chmod +x __htmgo/tailwind

RUN go run github.com/maddalax/htmgo/cli/htmgo@latest build

# Final stage
FROM alpine AS final

COPY --from=builder /src/dist/cal-anon-proxy /cal-anon-proxy

ENTRYPOINT ["/cal-anon-proxy"]
CMD ["server"]
