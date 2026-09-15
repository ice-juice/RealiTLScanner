package scan

import "net"

const (
	_ = iota
	// HostTypeIP is a single IP address target.
	HostTypeIP
	// HostTypeCIDR is an address expanded from a CIDR range.
	HostTypeCIDR
	// HostTypeDomain is a domain name that still needs DNS resolution.
	HostTypeDomain
)

// HostType classifies a generated scan target.
type HostType int

// Host is a single generated scan target.
type Host struct {
	IP     net.IP
	Origin string
	Type   HostType
}
