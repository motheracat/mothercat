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
		clamav-daemon \
		clamav-freshclam \
		netcat-openbsd \
	&& rm -rf /var/lib/apt/lists/* \
	&& mkdir -p /run/clamav /var/log/clamav \
	&& chown clamav:clamav /run/clamav /var/log/clamav /var/lib/clamav
COPY --from=build /out/mothercat-api /mothercat-api
COPY --from=build /out/setup-postgres /setup-postgres
COPY services/monolith/deploy/clamav/clamd.conf /etc/clamav/clamd.conf
COPY services/monolith/deploy/clamav/freshclam.conf /etc/clamav/freshclam.conf
COPY services/monolith/deploy/start-with-clamav /usr/local/bin/start-with-clamav

RUN chmod 755 /usr/local/bin/start-with-clamav
ENV SIWES_CLAMAV_ADDR=127.0.0.1:3310
ENTRYPOINT ["/usr/local/bin/start-with-clamav"]
EXPOSE 8080
STOPSIGNAL SIGTERM
CMD ["/mothercat-api"]
