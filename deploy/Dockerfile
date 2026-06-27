# syntax=docker/dockerfile:1

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/opentag ./cmd/opentag

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/opentag /opentag
USER nonroot:nonroot
ENTRYPOINT ["/opentag"]
CMD ["-config", "/etc/opentag/config.yaml"]
