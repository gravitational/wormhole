FROM ubuntu:18.04

ADD assets/docker/wireguard/wireguard_ubuntu_wireguard.gpg /etc/apt/trusted.gpg.d/wireguard_ubuntu_wireguard.gpg
ADD assets/docker/wireguard/wireguard-ubuntu-wireguard-bionic.list /etc/apt/sources.list.d/wireguard-ubuntu-wireguard-bionic.list

RUN apt-get update && \
    apt-get install --no-install-recommends -y wireguard && \
    rm -rf /var/lib/apt/lists/*

ADD build/wormhole /wormhole

CMD ["/wormhole"]  