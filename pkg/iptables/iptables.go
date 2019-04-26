/*
Copyright 2018 Gravitational, Inc.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package iptables

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"

	"github.com/coreos/go-iptables/iptables"
	"github.com/gravitational/trace"
)

var (
	WormholeAntispoofingChain = chain{filter, "WORMHOLE-ANTISPOOFING"}
	WormholeMSSChain          = chain{mangle, "WORMHOLE-MSS"}
)

type chain struct {
	table string
	name  string
}

type Config struct {
	logrus.FieldLogger

	// OverlayCIDR is the overlay network range
	OverlayCIDR string
	// PodCIDR is the local pod network range
	PodCIDR string

	// WireguardIface is the interface name for wireguard
	WireguardIface string
	// BridgeIface is the bridge interface name for the linux bridge
	BridgeIface string
	// SyncInterval is the time duration for resyncing the iptables rules on the host
	SyncInterval time.Duration

	iptables *iptables.IPTables
}

// Run will run the iptables control loop in a separate goroutine, that exits when the context is cancelled
func (c *Config) Run(ctx context.Context) error {
	ipt, err := iptables.New()
	if err != nil {
		return trace.Wrap(err)
	}

	if c.SyncInterval == 0 {
		return trace.BadParameter("Sync interval must be set")
	}

	c.iptables = ipt

	// cleanup rules that may have existed before and we crashed
	c.cleanupRules()
	// create the rules, so we can return any errors if there is a config problem
	err = c.createRules()
	if err != nil {
		return trace.Wrap(err)
	}

	go c.sync(ctx)

	return nil
}

func (c *Config) sync(ctx context.Context) {
	ticker := time.NewTicker(c.SyncInterval)
	defer ticker.Stop()
	defer c.cleanupRules()

	for {
		select {
		case <-ticker.C:
			err := c.rulesOk()
			if err != nil && !trace.IsNotFound(err) {
				c.Warn("Error checking iptables rules: ", trace.DebugReport(err))
				continue
			}
			if trace.IsNotFound(err) {
				// rules appear to be missing
				// so we delete then recreate our rules
				c.cleanupRules()

				err = c.createRules()
				if err != nil {
					c.Warn("Error creating iptables rules: ", trace.DebugReport(err))
				}
			}
			c.Debug("Iptables re-sync complete.")
		case <-ctx.Done():
			return
		}
	}
}

const (
	postrouting = "POSTROUTING"
	nat         = "nat"
	filter      = "filter"
	mangle      = "mangle"
	forward     = "FORWARD"
	input       = "INPUT"
)

func (c *Config) generateRules(links []netlink.Link) []rule {
	rules := make([]rule, 0)

	// Don't nat any traffic with source and destination within the overlay network
	rules = append(rules,
		rule{nat, postrouting, []string{"-s", c.OverlayCIDR, "-d", c.OverlayCIDR, "-j", "RETURN"},
			"wormhole: overlay->overlay"},
	)

	// Nat all other traffic
	if c.iptables.HasRandomFully() {
		rules = append(rules,
			rule{nat, postrouting, []string{"-s", c.PodCIDR, "-j", "MASQUERADE", "--random-fully"},
				"wormhole: nat overlay->internet"},
		)
	} else {
		rules = append(rules,
			rule{nat, postrouting, []string{"-s", c.PodCIDR, "-j", "MASQUERADE"},
				"wormhole: nat overlay->internet"},
		)
	}

	// Don't nat traffic from external hosts to local pods (preserves source IP when using externalTrafficPolicy=local)
	rules = append(rules,
		rule{nat, postrouting, []string{"-d", c.PodCIDR, "-j", "RETURN"},
			"wormhole: preserve source-ip"},
	)

	// Masquerade traffic if we're forwarding it to another host
	if c.iptables.HasRandomFully() {
		rules = append(rules,
			rule{nat, postrouting, []string{"-d", c.OverlayCIDR, "-j", "MASQUERADE", "--random-fully"},
				"wormhole: nat internet->overlay"},
		)
	} else {
		rules = append(rules,
			rule{nat, postrouting, []string{"-d", c.OverlayCIDR, "-j", "MASQUERADE"},
				"wormhole: nat internet->overlay"},
		)
	}

	//
	// Anti-spoofing, prevent tricking a host into routing traffic, if received on an unexpected interface
	// Traffic that is from the overlay network range, should only have source interfaces of the linux
	// bridge / wireguard / lo interfaces. Traffic entering on any other interface should be dropped.
	//
	// TODO(knisbet) look into whether it's possible for one pod to spoof another pod on the same host, and
	// whether we need iptable rules per pod (veth entry) to prevent this.

	rules = append(rules,
		rule{filter, WormholeAntispoofingChain.name, []string{"-i", c.BridgeIface, "-s", c.PodCIDR, "-j", "RETURN"},
			"wormhole: antispoofing"},
		// Wireguard will enforce the source address per peer, so just allow everything in the range
		rule{filter, WormholeAntispoofingChain.name, []string{"-i", c.WireguardIface, "-s", c.OverlayCIDR, "-j", "RETURN"},
			"wormhole: antispoofing"},
		// TODO: why is traffic getting marked to the local interface when testing locally
		rule{filter, WormholeAntispoofingChain.name, []string{"-i", "lo", "-j", "RETURN"},
			"wormhole: antispoofing"},
		rule{filter, WormholeAntispoofingChain.name, []string{"-j", "DROP"},
			"wormhole: drop spoofed traffic"},
	)

	// Apply anti-spoofing to the Forward / Input chains
	rules = append(rules,
		rule{filter, forward, []string{"-s", c.OverlayCIDR, "-j", WormholeAntispoofingChain.name},
			"wormhole: check antispoofing"},
		rule{filter, input, []string{"-s", c.OverlayCIDR, "-j", WormholeAntispoofingChain.name},
			"wormhole: check antispoofing"},
	)

	//
	// MSS Clamping
	// Set rules so that traffic that originates from the overlay network towards the internet
	// uses the MSS value of the host interface.

	for _, link := range links {
		if strings.HasPrefix(link.Attrs().Name, "wormhole") ||
			strings.HasPrefix(link.Attrs().Name, "veth") ||
			strings.HasPrefix(link.Attrs().Name, "lo") {
			// don't create clamping rules for wormhole / veth / local interfaces
			continue
		}
		rules = append(rules,
			rule{mangle, WormholeMSSChain.name, []string{"-o", link.Attrs().Name,
				"-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
				"-j", "TCPMSS", "--set-mss", fmt.Sprint(link.Attrs().MTU - 40)}, // 40 bytes below MTU for TCPv4
				"wormhole: mss clamping"},
		)
	}

	// link mangle forward table to mss chain
	rules = append(rules,
		rule{mangle, forward, []string{"-j", WormholeMSSChain.name},
			"wormhole: check mss clamping"},
	)

	return rules
}

func (c *Config) rulesOk() error {
	links, err := netlink.LinkList()
	if err != nil {
		return trace.Wrap(err)
	}

	for _, rule := range c.generateRules(links) {
		exists, err := c.iptables.Exists(rule.table, rule.chain, rule.getRule()...)
		if err != nil {
			return trace.Wrap(err)
		}
		if !exists {
			return trace.NotFound("missing rule")
		}
	}
	return nil
}

func (c *Config) cleanupRules() {
	for _, rule := range c.generateRules([]netlink.Link{}) {
		c.Info("Deleting iptables rule: table: ", rule.table, " chain: ", rule.chain, " spec: ",
			strings.Join(rule.getRule(), " "))

		// ignore and log errors in delete, which are likely caused by the rule not existing
		err := c.iptables.Delete(rule.table, rule.chain, rule.getRule()...)
		if err != nil {
			c.Info("Delete rule failed: ", err)
		}
	}

	for _, chain := range []chain{WormholeAntispoofingChain, WormholeMSSChain} {
		err := c.iptables.ClearChain(chain.table, chain.name)
		if err != nil {
			c.Info("Clear chain ", chain, " failed: ", err)
		}

		err = c.iptables.DeleteChain(chain.table, chain.name)
		if err != nil {
			c.Info("Delete chain ", chain, " failed: ", err)
		}
	}
}

func (c *Config) createRules() error {

	for _, chain := range []chain{WormholeAntispoofingChain, WormholeMSSChain} {
		err := c.iptables.ClearChain(chain.table, chain.name)
		if err != nil {
			c.Info("Clear chain ", chain, " failed: ", err)
		}

		err = c.iptables.DeleteChain(chain.table, chain.name)
		if err != nil {
			c.Info("Delete chain ", chain, " failed: ", err)
		}

		err = c.iptables.NewChain(chain.table, chain.name)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	links, err := netlink.LinkList()
	if err != nil {
		return trace.Wrap(err)
	}

	for _, rule := range c.generateRules(links) {
		c.Info("Adding iptables rule: table: ", rule.table, " chain: ", rule.chain, " spec: ",
			strings.Join(rule.getRule(), " "))

		err = c.iptables.AppendUnique(rule.table, rule.chain, rule.getRule()...)
		if err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

type rule struct {
	table   string
	chain   string
	rule    []string
	comment string
}

func (r rule) getRule() []string {
	comment := []string{"-m", "comment", "--comment", r.comment}
	return append(r.rule, comment...)
}
