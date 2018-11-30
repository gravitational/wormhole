FROM ubuntu:18.10 as wireguard
ADD assets/docker/wireguard/wireguard_ubuntu_wireguard.gpg /etc/apt/trusted.gpg.d/wireguard_ubuntu_wireguard.gpg
ADD assets/docker/wireguard/wireguard-ubuntu-wireguard-bionic.list /etc/apt/sources.list.d/wireguard-ubuntu-wireguard-bionic.list

RUN apt-get update && \
    apt-get install --no-install-recommends -y \
    wireguard

FROM quay.io/gravitational/rig:5.3.1 as rig


FROM ubuntu:18.10

ARG CNI_VERSION
ARG ARCH

RUN apt-get update && \
    apt-get install --no-install-recommends -y \
    iproute2 \
    net-tools \
    iptables \
    curl \
    ca-certificates \
    && update-ca-certificates \
    && rm -rf /var/lib/apt/lists/*

#
# Install/Upgrade/Rollback interactions for a gravity cluster
#
ARG CHANGESET
ENV RIG_CHANGESET $CHANGESET
COPY --from=rig /usr/local/bin/rig /usr/bin/rig
ADD docs/gravity-wormhole.yaml /gravity/wormhole.yaml
ADD scripts/gravity* /gravity/
RUN chmod +x /gravity/gravity*


COPY --from=wireguard /usr/bin/wg /usr/bin/wg

# Get a copy of CNI plugins, so we can copy them to the host
RUN mkdir -p /opt/cni/bin
RUN curl -L --retry 5 https://github.com/containernetworking/plugins/releases/download/${CNI_VERSION}/cni-plugins-${ARCH}-${CNI_VERSION}.tgz \
    | tar -xz -C /opt/cni/bin ./bridge ./loopback ./host-local ./portmap ./tuning

ADD build/wormhole /wormhole

CMD ["/wormhole"]  