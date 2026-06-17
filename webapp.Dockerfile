FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /webapp ./webapp

FROM gcr.io/distroless/static-debian12
COPY --from=builder /webapp /webapp
EXPOSE 8080
ENTRYPOINT ["/webapp"]
