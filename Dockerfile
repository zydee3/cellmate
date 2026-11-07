FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /cellmate ./cmd/cellmate

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /cellmate /cellmate
ENV DATA_FILE=/data/cellmate.json
EXPOSE 8080
ENTRYPOINT ["/cellmate"]
