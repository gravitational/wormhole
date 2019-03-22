ARG BUILD_IMAGE
FROM ${BUILD_IMAGE}

ARG GOLANGCI_VER

RUN env
RUN curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh| sh -s -- -b $GOPATH/bin ${GOLANGCI_VER}

#ADD docker/wireguard/wireguard_ubuntu_wireguard.gpg /etc/apt/trusted.gpg.d/wireguard_ubuntu_wireguard.gpg
#ADD docker/wireguard/wireguard-ubuntu-wireguard-bionic.list /etc/apt/sources.list.d/wireguard-ubuntu-wireguard-bionic.list

RUN echo "deb http://deb.debian.org/debian/ unstable main" > /etc/apt/sources.list.d/unstable.list
RUN apt-get update && \
    apt-get install --no-install-recommends -y \
    wireguard

WORKDIR "/go/src/github.com/gravitational/wormhole/"