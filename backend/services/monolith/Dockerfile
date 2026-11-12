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
	./services/monolith/cmd/api \
	&& CGO_ENABLED=0 GOOS=linux go build \
		-trimpath \
		-ldflags="-s -w" \
		-o /out/setup-postgres \
		./services/monolith/cmd/setup-postgres

FROM debian:bookworm-slim
RUN apt-get update \
	&& apt-get install -y --no-install-recommends \
		ca-certificates \
	&& rm -rf /var/lib/apt/lists/*
COPY --from=build /out/mothercat-api /mothercat-api
COPY --from=build /out/setup-postgres /setup-postgres

USER nobody
EXPOSE 8080
STOPSIGNAL SIGTERM
CMD ["/mothercat-api"]
