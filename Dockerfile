FROM golang:1.22-alpine AS builder

ARG SERVICE=api-gateway

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/service ./cmd/${SERVICE}

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
COPY --from=builder /bin/service /usr/local/bin/service
COPY migrations/ /app/migrations/

ENTRYPOINT ["service"]
