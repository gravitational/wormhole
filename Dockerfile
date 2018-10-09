FROM ubuntu:18.04

ADD build/wormhole /wormhole

CMD ["/wormhole"]  