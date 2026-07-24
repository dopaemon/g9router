FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/g9router ./cmd/g9router

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/g9router /g9router
EXPOSE 20128
ENTRYPOINT ["/g9router"]
