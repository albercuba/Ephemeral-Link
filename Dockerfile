FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ephemeral-link ./cmd/server

FROM alpine:3.21
RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/ephemeral-link /app/ephemeral-link
COPY web /app/web
COPY locales /app/locales
RUN mkdir -p /app/data/storage && chown -R app:app /app
USER app
EXPOSE 8080
ENTRYPOINT ["/app/ephemeral-link"]
