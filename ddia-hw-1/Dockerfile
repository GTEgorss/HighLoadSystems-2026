FROM golang:1.26.1-alpine3.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/server ./cmd/server && \
    CGO_ENABLED=0 go build -o /bin/generate ./cmd/generate && \
    CGO_ENABLED=0 go build -o /bin/load ./cmd/load && \
    CGO_ENABLED=0 go build -o /bin/setup ./cmd/setup

FROM alpine:3.23.3
RUN apk add --no-cache ca-certificates
COPY --from=build /bin/server /bin/generate /bin/load /bin/setup /usr/local/bin/
USER 10001:10001
CMD ["server"]
