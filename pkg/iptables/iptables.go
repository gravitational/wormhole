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
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/coreos/go-iptables/iptables"
	"github.com/gravitational/trace"
)

const (
	WormholeAntispoofingChain = "WORMHOLE-ANTISPOOFING"
)

type Config struct {
	logrus.FieldLogger

	// OverlayCIDR is the overlay network range
	OverlayCIDR string
	// PodCIDR is the local pod network range
	PodCIDR string

	WireguardIface string
	BridgeIface    string
	SyncInterval   time.Duration

	iptables *iptables.IPTables
}

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
	defer c.cleanupRules()

	for {
		select {
		case <-ticker.C:
			ok, err := c.rulesOk()
			if err != nil {
				c.Warn("Error checking iptables rules: ", trace.DebugReport(err))
				continue
			}
			if !ok {
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

func (c *Config) generateRules() []rule {
	rules := make([]rule, 0)

	// Don't nat any traffic with source and destination within the overlay network
	rules = append(rules,
		rule{"nat", "POSTROUTING", []string{"-s", c.OverlayCIDR, "-d", c.OverlayCIDR, "-j", "RETURN"},
			"wormhole: overlay->overlay"},
	)

	// Nat all other traffic
	if c.iptables.HasRandomFully() {
		rules = append(rules,
			rule{"nat", "POSTROUTING", []string{"-s", c.PodCIDR, "-j", "MASQUERADE", "--random-fully"},
				"wormhole: nat overlay->internet"},
		)
	} else {
		rules = append(rules,
			rule{"nat", "POSTROUTING", []string{"-s", c.PodCIDR, "-j", "MASQUERADE"},
				"wormhole: nat overlay->internet"},
		)
	}

	// Don't nat traffic from external hosts to local pods (preserves source IP when using externalTrafficPolicy=local)
	rules = append(rules,
		rule{"nat", "POSTROUTING", []string{"-d", c.PodCIDR, "-j", "RETURN"},
			"wormhole: preserve source-ip"},
	)

	// Masquerade traffic if we're forwarding it to another host
	if c.iptables.HasRandomFully() {
		rules = append(rules,
			rule{"nat", "POSTROUTING", []string{"-d", c.OverlayCIDR, "-j", "MASQUERADE", "--random-fully"},
				"wormhole: nat internet->overlay"},
		)
	} else {
		rules = append(rules,
			rule{"nat", "POSTROUTING", []string{"-d", c.OverlayCIDR, "-j", "MASQUERADE"},
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
		rule{"filter", WormholeAntispoofingChain, []string{"-i", c.BridgeIface, "-s", c.PodCIDR, "-j", "RETURN"},
			"wormhole: antispoofing"},
		// Wireguard will enforce the source address per peer, so just allow everything in the range
		rule{"filter", WormholeAntispoofingChain, []string{"-i", c.WireguardIface, "-s", c.OverlayCIDR, "-j", "RETURN"},
			"wormhole: antispoofing"},
		// TODO: why is traffic getting marked to the local interface when testing locally
		rule{"filter", WormholeAntispoofingChain, []string{"-i", "lo", "-j", "RETURN"},
			"wormhole: antispoofing"},
		rule{"filter", WormholeAntispoofingChain, []string{"-j", "DROP"},
			"wormhole: drop spoofed traffic"},
	)

	// Apply anti-spoofing to the Forward / Input chains
	rules = append(rules,
		rule{"filter", "FORWARD", []string{"-s", c.OverlayCIDR, "-j", WormholeAntispoofingChain},
			"wormhole: check antispoofing"},
		rule{"filter", "INPUT", []string{"-s", c.OverlayCIDR, "-j", WormholeAntispoofingChain},
			"wormhole: check antispoofing"},
	)

	return rules
}

func (c *Config) rulesOk() (bool, error) {
	for _, rule := range c.generateRules() {
		exists, err := c.iptables.Exists(rule.table, rule.chain, rule.getRule()...)
		if err != nil {
			return false, trace.Wrap(err)
		}
		if !exists {
			return false, nil
		}
	}
	return true, nil
}

func (c *Config) cleanupRules() {
	for _, rule := range c.generateRules() {
		c.Info("Deleting iptables rule: table: ", rule.table, " chain: ", rule.chain, " spec: ",
			strings.Join(rule.getRule(), " "))

		// ignore and log errors in delete, which are likely caused by the rule not existing
		err := c.iptables.Delete(rule.table, rule.chain, rule.getRule()...)
		if err != nil {
			c.Info("Delete rule failed: ", err)
		}
	}

	err := c.iptables.DeleteChain("filter", WormholeAntispoofingChain)
	if err != nil {
		c.Info("Delete chain ", WormholeAntispoofingChain, " failed: ", err)
	}
}

func (c *Config) createRules() error {
	err := c.iptables.ClearChain("filter", WormholeAntispoofingChain)
	if err != nil {
		return trace.Wrap(err)
	}

	for _, rule := range c.generateRules() {
		c.Info("Adding iptables rule: table: ", rule.table, " chain: ", rule.chain, " spec: ",
			strings.Join(rule.getRule(), " "))

		// ignore and log errors in delete, which are likely caused by the rule not existing
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
