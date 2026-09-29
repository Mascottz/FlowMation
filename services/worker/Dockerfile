FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod .
COPY main.go .
RUN go mod tidy && go build -o worker main.go

FROM alpine:3.20
COPY --from=build /src/worker /usr/local/bin/worker
EXPOSE 8082
ENTRYPOINT ["/usr/local/bin/worker"]
