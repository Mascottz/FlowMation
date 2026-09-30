FROM rust:1.81-alpine AS build
WORKDIR /src
COPY Cargo.toml .
COPY src ./src
RUN cargo build --release

FROM alpine:3.20
COPY --from=build /src/target/release/flowmation-engine /usr/local/bin/flowmation-engine
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/flowmation-engine"]
