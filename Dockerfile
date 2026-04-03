FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /goanimes ./cmd/goanimes

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /goanimes /usr/local/bin/goanimes
ENV GOANIMES_ADDR=:8080
ENV GOANIMES_DATA_DIR=/app/data
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/goanimes"]
