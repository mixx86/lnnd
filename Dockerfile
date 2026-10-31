FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o lnnd ./cmd/lnnd


FROM alpine:latest

WORKDIR /app
COPY --from=builder /app/lnnd .

EXPOSE 5771

VOLUME ["/root/.lnn"]

ENTRYPOINT ["./lnnd"]
