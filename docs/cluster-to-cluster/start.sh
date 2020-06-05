#!/usr/bin/env bash
set -x

# Setup Instructions
# 1. Create private key for a particular cluster and upload as kubernetes secret
#    umask 077 && wg genkey > wg0.key && kubectl -n wormhole create secret generic wg-cluster1-cluster2-privatekey --from-file=wg0.key
#    Note: the corresponding public key can be shown with: 
#      cat wg0.key | wg pubkey
#   
# 2. svc_ip: change kube-dns.kube-system.svc.cluster.local to be the DNS name of the service on this cluster to use
#            Connections that originate from the remote cluster, will be sent to this service on the local cluster
# 3. peer_public: Set this to the public key of the remote cluster
# 4. Comment/Uncomment wg0_ip/peer_wg0_ip/peer_endpoint on whether this is the central or remote cluster
#    We need to coordinate IP addresses used by wireguard, so each end of the connection get's unique configuration
#    Note: peer_endpoint is only needed on one end of the connection. In this case, we assume that the leaf cluster
#          is responsible for establishing the connection to the central cluster. It is safe to configure this on both
#          ends of the connection.
#    Note: peer endpoint must contain the IP/Port used to reach the central cluster. This example uses a nodePort of 32760
#          and an IP of 10.162.0.7. This connection should work through a NAT, however, from the central cluster, a unique
#          nodePort is required for each leaf cluster.
# 5. remote/local svc_port: This sets up NAT rules, for sending traffic between clusters. This is the port number, of the
#                           service to match when creating NAT rules.
# 6. Once the script has been configured, create it as a configmap for the particular cluster pair
#    kubectl -n wormhole create configmap wg-cluster1-cluster2-start --from-file=start.sh
#    where cluster1 = name of local cluster
#    where cluster2 = name of remote cluster
# 7. When up and running, delete the local copy of the private key from disk:
#    rm wg0.key

# Testing the example:
# leaf: 
#   dig @`kubectl -n wormhole get svc cluster1-kube-dns -o jsonpath="{.spec.clusterIP}"` gravity-site.kube-system.svc.cluster.local
#   dig @`kubectl -n wormhole get svc cluster1-kube-dns -o jsonpath="{.spec.clusterIP}"` gravity-site.kube-system.svc.cluster.local +tcp
# central:
#   dig @`kubectl -n wormhole get svc cluster2-kube-dns -o jsonpath="{.spec.clusterIP}"` gravity-site.kube-system.svc.cluster.local
#   dig @`kubectl -n wormhole get svc cluster2-kube-dns -o jsonpath="{.spec.clusterIP}"` gravity-site.kube-system.svc.cluster.local +tcp


svc_ip=`getent hosts kube-dns.kube-system.svc.cluster.local | cut -d ' ' -f 1`
peer_public=""
remote_svc_port=53
local_svc_port=53

#
# Central Cluster
#
wg0_ip=169.254.254.1
peer_wg0_ip=169.254.254.2
peer_endpoint=""

#
# Remote Cluster
#
#wg0_ip=169.254.254.2
#peer_wg0_ip=169.254.254.1
#peer_endpoint="endpoint 10.162.0.7:32760"

# Setup wireguard tunnel to peer cluster
ip link add dev wg0 type wireguard
ip address add dev wg0 ${wg0_ip}/24
wg set wg0 listen-port 51820 private-key /etc/wg0-private/wg0.key
ip link set up dev wg0



# Setup NAT rules
# Traffic from local cluster, DNAT to remote cluster wg0, SNAT to wg0
iptables -t nat -A PREROUTING -p tcp -m tcp -d ${POD_IP}/32 --destination-port ${remote_svc_port} -j DNAT --to-destination ${peer_wg0_ip}
iptables -t nat -A POSTROUTING -p tcp -m tcp -d ${peer_wg0_ip}/32 --destination-port ${remote_svc_port} -j SNAT --to-source ${wg0_ip}
iptables -t nat -A PREROUTING -p udp -m udp -d ${POD_IP}/32 --destination-port ${remote_svc_port} -j DNAT --to-destination ${peer_wg0_ip}
iptables -t nat -A POSTROUTING -p udp -m udp -d ${peer_wg0_ip}/32 --destination-port ${remote_svc_port} -j SNAT --to-source ${wg0_ip}

# Traffic from remote cluster, DNAT to service, SNAT to POD_IP
iptables -t nat -A PREROUTING -p tcp -m tcp -d ${wg0_ip}/32  --destination-port ${local_svc_port} -j DNAT --to-destination ${svc_ip}
iptables -t nat -A POSTROUTING -p tcp -m tcp -d ${svc_ip}/32 --destination-port ${local_svc_port} -j SNAT --to-source ${POD_IP}
iptables -t nat -A PREROUTING -p udp -m udp -d ${wg0_ip}/32  --destination-port ${local_svc_port} -j DNAT --to-destination ${svc_ip}
iptables -t nat -A POSTROUTING -p udp -m udp -d ${svc_ip}/32 --destination-port ${local_svc_port} -j SNAT --to-source ${POD_IP}

# Create peer entry within WireGuard
wg set wg0 peer ${peer_public} allowed-ips ${peer_wg0_ip}/32 ${peer_endpoint}

# block execution and keep the pod running forever
echo "Setup complete, sleeping..."
sleep infinity