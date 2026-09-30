FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod .
COPY main.go storage.go queue.go .
RUN go mod download && go build -o gateway main.go storage.go queue.go

FROM alpine:3.20
COPY --from=build /src/gateway /usr/local/bin/gateway
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/gateway"]
