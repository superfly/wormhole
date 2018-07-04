FROM alpine

RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*

ADD app /
ENTRYPOINT ["/app"]
