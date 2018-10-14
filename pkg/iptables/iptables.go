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

	"github.com/cloudflare/cfssl/log"
	"github.com/sirupsen/logrus"

	"github.com/coreos/go-iptables/iptables"
	"github.com/gravitational/trace"
)

type Config struct {
	logrus.FieldLogger

	// OverlayCIDR is the overlay network range
	OverlayCIDR string
	// PodCIDR is the local pod network range
	PodCIDR string

	iptables *iptables.IPTables
}

func (c *Config) Run(ctx context.Context) error {
	ipt, err := iptables.New()
	if err != nil {
		return trace.Wrap(err)
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
	ticker := time.NewTicker(15 * time.Second)
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
			c.Info("Iptables re-sync complete.")
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
			"wormhole - don't nat overlay traffic"},
	)

	// Nat all other traffic
	if c.iptables.HasRandomFully() {
		rules = append(rules,
			rule{"nat", "POSTROUTING", []string{"-s", c.PodCIDR, "-j", "MASQUERADE", "--random-fully"},
				"wormhole - nat pod traffic leaving the host"},
		)
	} else {
		rules = append(rules,
			rule{"nat", "POSTROUTING", []string{"-s", c.PodCIDR, "-j", "MASQUERADE"},
				"wormhole - nat pod traffic leaving the host"},
		)
	}

	// Don't nat traffic from external hosts to local pods (preserves source IP when using externalTrafficPolicy=local)
	rules = append(rules,
		rule{"nat", "POSTROUTING", []string{"-d", c.PodCIDR, "-j", "RETURN"},
			"wormhole - preserve source ip for local pods"},
	)

	// Masquerade traffic if we're forwarding it to another host
	if c.iptables.HasRandomFully() {
		rules = append(rules,
			rule{"nat", "POSTROUTING", []string{"-d", c.OverlayCIDR, "-j", "MASQUERADE", "--random-fully"},
				"wormhole - nat overlay traffic"},
		)
	} else {
		rules = append(rules,
			rule{"nat", "POSTROUTING", []string{"-d", c.OverlayCIDR, "-j", "MASQUERADE"},
				"wormhole - nat overlay traffic"},
		)
	}

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
		log.Info("Deleting iptables rule: table: ", rule.table, " chain: ", rule.chain, " spec: ",
			strings.Join(rule.getRule(), " "))

		// ignore and log errors in delete, which are likely caused by the rule not existing
		err := c.iptables.Delete(rule.table, rule.chain, rule.getRule()...)
		if err != nil {
			c.Info("Delete rule failed: ", err)
		}
	}
}

func (c *Config) createRules() error {
	for _, rule := range c.generateRules() {
		log.Info("Adding iptables rule: table: ", rule.table, " chain: ", rule.chain, " spec: ",
			strings.Join(rule.getRule(), " "))

		// ignore and log errors in delete, which are likely caused by the rule not existing
		err := c.iptables.AppendUnique(rule.table, rule.chain, rule.getRule()...)
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
