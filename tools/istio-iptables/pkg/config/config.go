// Copyright 2019 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"encoding/json"
	"fmt"
	"time"

	"istio.io/pkg/log"
)

// Command line options
// nolint: maligned
type Config struct {
	ProxyPort               string        `json:"PROXY_PORT,omitempty"`
	InboundCapturePort      string        `json:"INBOUND_CAPTURE_PORT,omitempty"`
	ProxyUID                string        `json:"PROXY_UID,omitempty"`
	ProxyGID                string        `json:"PROXY_GID,omitempty"`
	InboundInterceptionMode string        `json:"INBOUND_INTERCEPTION_MODE,omitempty"`
	InboundTProxyMark       string        `json:"INBOUND_TPROXY_MARK,omitempty"`
	InboundTProxyRouteTable string        `json:"INBOUND_TPROXY_ROUTE_TABLE,omitempty"`
	InboundPortsInclude     string        `json:"INBOUND_PORTS_INCLUDE,omitempty"`
	InboundPortsExclude     string        `json:"INBOUND_PORTS_EXCLUDE,omitempty"`
	OutboundPortsExclude    string        `json:"OUTBOUND_PORTS_EXCLUDE,omitempty"`
	OutboundIPRangesInclude string        `json:"OUTBOUND_IPRANGES_INCLUDE,omitempty"`
	OutboundIPRangesExclude string        `json:"OUTBOUND_IPRANGES_EXCLUDE,omitempty"`
	KubevirtInterfaces      string        `json:"KUBEVIRT_INTERFACES,omitempty"`
	IptablesProbePort       uint16        `json:"IPTABLES_PROBE_PORT,omitempty"`
	ProbeTimeout            time.Duration `json:"PROBE_TIMEOUT,omitempty"`
	DryRun                  bool          `json:"DRY_RUN,omitempty"`
	RestoreFormat           bool          `json:"RESTORE_FORMAT,omitempty"`
	SkipRuleApply           bool          `json:"SKIP_RULE_APPLY,omitempty"`
	RunValidation           bool          `json:"RUN_VALIDATION,omitempty"`
	EnableInboundIPv6       bool          `json:"ENABLE_INBOUND_IPV6,omitempty"`
}

func (c *Config) String() string {
	output, err := json.MarshalIndent(c, "", "\t")
	if err != nil {
		log.Fatalf("Unable to marshal config object: %v", err)
	}
	return string(output)
}

func (c *Config) Print() {
	fmt.Println("Variables:")
	fmt.Println("----------")
	fmt.Printf("PROXY_PORT=%s\n", c.ProxyPort)
	fmt.Printf("PROXY_INBOUND_CAPTURE_PORT=%s\n", c.InboundCapturePort)
	fmt.Printf("PROXY_UID=%s\n", c.ProxyUID)
	fmt.Printf("PROXY_GID=%s\n", c.ProxyGID)
	fmt.Printf("INBOUND_INTERCEPTION_MODE=%s\n", c.InboundInterceptionMode)
	fmt.Printf("INBOUND_TPROXY_MARK=%s\n", c.InboundTProxyMark)
	fmt.Printf("INBOUND_TPROXY_ROUTE_TABLE=%s\n", c.InboundTProxyRouteTable)
	fmt.Printf("INBOUND_PORTS_INCLUDE=%s\n", c.InboundPortsInclude)
	fmt.Printf("INBOUND_PORTS_EXCLUDE=%s\n", c.InboundPortsExclude)
	fmt.Printf("OUTBOUND_IP_RANGES_INCLUDE=%s\n", c.OutboundIPRangesInclude)
	fmt.Printf("OUTBOUND_IP_RANGES_EXCLUDE=%s\n", c.OutboundIPRangesExclude)
	fmt.Printf("OUTBOUND_PORTS_EXCLUDE=%s\n", c.OutboundPortsExclude)
	fmt.Printf("KUBEVIRT_INTERFACES=%s\n", c.KubevirtInterfaces)
	fmt.Printf("ENABLE_INBOUND_IPV6=%t\n", c.EnableInboundIPv6)
	fmt.Println("")
}
