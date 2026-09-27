FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api

# The binary is static (CGO disabled), so it needs no base image.
FROM scratch
WORKDIR /app
COPY --from=build /out/api /app/api
COPY config /app/config
COPY fixtures /app/fixtures
EXPOSE 8080
ENTRYPOINT ["/app/api"]
