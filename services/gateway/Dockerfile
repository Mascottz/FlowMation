FROM golang:1.23-alpine AS build
WORKDIR /src
COPY main.go .
RUN go build -o gateway main.go

FROM alpine:3.20
COPY --from=build /src/gateway /usr/local/bin/gateway
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/gateway"]
