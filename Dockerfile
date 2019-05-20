# Builder Image
FROM golang:1.12.5-alpine3.9 as builder

RUN apk update \
    && apk upgrade \
    && apk add --no-cache git bash make

WORKDIR /triebwerk

COPY Makefile go.mod go.sum ./

RUN make tools

COPY . ./

RUN chmod +x docker/docker-entrypoint.sh \
    && make build-static

# Run image

FROM alpine:3.9

WORKDIR /

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /triebwerk/docker/docker-entrypoint.sh .
COPY --from=builder /triebwerk/triebwerk .

ENTRYPOINT ["/docker-entrypoint.sh"]

CMD ["/triebwerk"]
