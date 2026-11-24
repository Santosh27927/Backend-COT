FROM golang:1.27.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG SERVICE=game-service
RUN case "$SERVICE" in game-service|trading-service|migrate) ;; *) exit 2 ;; esac && CGO_ENABLED=0 go build -trimpath -o /out/service "./cmd/$SERVICE"

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && addgroup -S cot && adduser -S -G cot cot
COPY --from=build /out/service /usr/local/bin/cot-service
USER cot
ENTRYPOINT ["/usr/local/bin/cot-service"]
