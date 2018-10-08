FROM golang:1.11.1
WORKDIR /go/src/github.com/gravitational/wormhole/
COPY ./ /go/src/github.com/gravitational/wormhole/
RUN go run mage.go build

FROM ubuntu:18.04
COPY --from=0 /go/src/github.com/gravitational/wormhole/build/wormhole /
CMD ["./wormhole daemon"]  