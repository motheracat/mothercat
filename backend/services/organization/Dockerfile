ARG GO_VERSION=1.27.3
FROM golang:${GO_VERSION}-alpine AS build

WORKDIR /src
RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
	-trimpath \
	-ldflags="-s -w" \
	-o /out/organization-api \
	./cmd/api

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/organization-api /organization-api

USER 65532:65532
EXPOSE 8080
STOPSIGNAL SIGTERM
ENTRYPOINT ["/organization-api"]