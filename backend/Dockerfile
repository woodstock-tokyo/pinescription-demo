FROM golang:1.26-alpine AS builder
WORKDIR /app
# Copy dependency files first for layer caching
ENV GOPRIVATE=github.com/woodstock-tokyo/pinescription
RUN apk add --no-cache git openssh-client ca-certificates
RUN mkdir -p -m 0700 /root/.ssh && \
    ssh-keyscan github.com >> /root/.ssh/known_hosts
COPY go.mod go.sum ./
RUN --mount=type=ssh go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /server .
EXPOSE 8080
CMD ["./server"]
