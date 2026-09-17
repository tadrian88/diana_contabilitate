FROM golang:1.26-alpine AS build
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api && CGO_ENABLED=0 go build -o /out/worker ./cmd/worker && CGO_ENABLED=0 go build -o /out/authuser ./cmd/authuser

FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 diana
COPY --from=build /out/ /usr/local/bin/
ENV TZ=UTC
USER diana
CMD ["api"]
