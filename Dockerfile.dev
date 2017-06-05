FROM golang:1.7

COPY . /go/src/github.com/superfly/wormhole

RUN go get github.com/valyala/fasthttp

ENTRYPOINT ["go", "run", "/go/src/github.com/superfly/wormhole/cmd/wormhole/main.go"]
