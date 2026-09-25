FROM golang:1.27.0-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /limiter ./cmd/limiter

FROM alpine:3.21
COPY --from=build /limiter /usr/local/bin/limiter
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/limiter"]
