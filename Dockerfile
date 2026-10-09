FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN go mod tidy && CGO_ENABLED=0 go build -o /cinebot ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D app
USER app
COPY --from=build /cinebot /cinebot
ENV PORT=8080
EXPOSE 8080
CMD ["/cinebot"]
