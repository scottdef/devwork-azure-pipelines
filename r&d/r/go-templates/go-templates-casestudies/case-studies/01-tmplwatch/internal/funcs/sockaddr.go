package funcs

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Iface describes one network interface for GetAllInterfaces.
type Iface struct {
	Name     string
	Index    int
	MTU      int
	Up       bool
	Loopback bool
	Addrs    []string // CIDR form
}

// ips returns every interface address in system order, skipping
// interfaces that are down.
func ips() ([]net.IP, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []net.IP
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok {
				out = append(out, n.IP)
			}
		}
	}
	return out, nil
}

// pick returns the first IPv4 address satisfying ok, else the first IPv6.
func pick(all []net.IP, ok func(net.IP) bool) string {
	var v6 string
	for _, ip := range all {
		if !ok(ip) {
			continue
		}
		if ip.To4() != nil {
			return ip.String()
		}
		if v6 == "" {
			v6 = ip.String()
		}
	}
	return v6
}

// GetPrivateIP returns the first RFC 1918 / RFC 4193 address, preferring
// IPv4, or "" if there is none.
func GetPrivateIP() (string, error) {
	all, err := ips()
	if err != nil {
		return "", fmt.Errorf("GetPrivateIP: %w", err)
	}
	return pick(all, func(ip net.IP) bool { return ip.IsPrivate() }), nil
}

// GetPublicIP returns the first global unicast address that is not
// private, preferring IPv4, or "" if there is none.
func GetPublicIP() (string, error) {
	all, err := ips()
	if err != nil {
		return "", fmt.Errorf("GetPublicIP: %w", err)
	}
	return pick(all, func(ip net.IP) bool { return ip.IsGlobalUnicast() && !ip.IsPrivate() }), nil
}

// GetInterfaceIP returns the first address of the named interface,
// preferring IPv4.
func GetInterfaceIP(name string) (string, error) {
	ifc, err := net.InterfaceByName(name)
	if err != nil {
		return "", fmt.Errorf("GetInterfaceIP: %w", err)
	}
	addrs, err := ifc.Addrs()
	if err != nil {
		return "", fmt.Errorf("GetInterfaceIP: %w", err)
	}
	var all []net.IP
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			all = append(all, n.IP)
		}
	}
	ip := pick(all, func(net.IP) bool { return true })
	if ip == "" {
		return "", errors.New("GetInterfaceIP: " + name + " has no addresses")
	}
	return ip, nil
}

// GetAllInterfaces lists every interface with its addresses.
func GetAllInterfaces() ([]Iface, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("GetAllInterfaces: %w", err)
	}
	out := make([]Iface, 0, len(ifs))
	for _, ifc := range ifs {
		i := Iface{Name: ifc.Name, Index: ifc.Index, MTU: ifc.MTU,
			Up: ifc.Flags&net.FlagUp != 0, Loopback: ifc.Flags&net.FlagLoopback != 0}
		if addrs, err := ifc.Addrs(); err == nil {
			for _, a := range addrs {
				i.Addrs = append(i.Addrs, a.String())
			}
		}
		out = append(out, i)
	}
	return out, nil
}

// Sockaddr evaluates the small subset of go-sockaddr expressions that
// consul-template configurations use in practice.
func Sockaddr(expr string) (string, error) {
	f := strings.Fields(strings.TrimSpace(expr))
	switch {
	case len(f) == 1 && f[0] == "GetPrivateIP":
		return GetPrivateIP()
	case len(f) == 1 && f[0] == "GetPublicIP":
		return GetPublicIP()
	case len(f) == 2 && f[0] == "GetInterfaceIP":
		name, err := strconv.Unquote(f[1])
		if err != nil {
			return "", fmt.Errorf("sockaddr: interface name must be quoted: %s", f[1])
		}
		return GetInterfaceIP(name)
	}
	return "", fmt.Errorf("sockaddr: unsupported expression %q", expr)
}
