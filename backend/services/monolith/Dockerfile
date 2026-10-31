ARG GO_VERSION=1.27.3
FROM golang:${GO_VERSION}-alpine AS build

WORKDIR /src
RUN apk add --no-cache ca-certificates

COPY go.work go.work.sum ./
COPY services/ ./services/

RUN CGO_ENABLED=0 GOOS=linux go build \
	-trimpath \
	-ldflags="-s -w" \
	-o /out/mothercat-api \
	./services/monolith/cmd/api

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/mothercat-api /mothercat-api

USER 65532:65532
EXPOSE 8080
STOPSIGNAL SIGTERM
ENTRYPOINT ["/mothercat-api"]
