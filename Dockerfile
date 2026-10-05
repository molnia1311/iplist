# Multi-stage build. Static binary, minimal runtime image.

ARG GO_VERSION=1.23

FROM golang:${GO_VERSION}-alpine AS build
ARG BINARY=iplist
WORKDIR /src

# Cache dependencies separately from source
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/${BINARY} ./...

FROM gcr.io/distroless/static-debian12:nonroot
ARG BINARY=iplist
COPY --from=build /out/${BINARY} /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
