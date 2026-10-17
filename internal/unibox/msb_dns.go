package unibox

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
)

// msb on macOS asks only the global DNS servers (State:/Network/Global/DNS)
// and does no split DNS: a name a VPN resolves through a resolver of its
// own, such as an internal forge, does not resolve in the sandbox. msb also
// drops a DNS answer with a private address unless a rule names that
// address. And it puts a secret into a request only to an address it saw a
// DNS answer for, so the name cannot be pinned in /etc/hosts instead.
// Checked against msb 0.7.8 on macOS with a split-DNS VPN.
//
// So before a sandbox is made unibox resolves the hosts its profile names,
// as the host resolves them. For a host with a private address it finds the
// host's resolver that gives that address and puts it before the global
// ones, and allows that address. msb moves to the next resolver only when
// one does not answer, so the resolver has to answer public names as well.
// What it finds holds until the sandbox is recreated.
type hostDNS struct {
	// lookup resolves a name as the host does.
	lookup func(ctx context.Context, name string) ([]net.IP, error)
	// query asks one resolver for a name.
	query func(ctx context.Context, server, name string) ([]net.IP, error)
	// resolvers lists the host's resolvers: the global ones, then every
	// other one it knows, in its order.
	resolvers func(ctx context.Context) (global, all []string, err error)
}

// msbDNSProbe is a public name the chosen resolver must answer as well, as
// msb falls through to the next resolver only when one does not answer.
const msbDNSProbe = "github.com"

func defaultHostDNS() *hostDNS {
	return &hostDNS{
		lookup: func(ctx context.Context, name string) ([]net.IP, error) {
			addrs, err := net.DefaultResolver.LookupIPAddr(ctx, name)
			ips := []net.IP{}
			for _, a := range addrs {
				ips = append(ips, a.IP)
			}
			return ips, err
		},
		query: func(ctx context.Context, server, name string) ([]net.IP, error) {
			r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 2 * time.Second}
				return d.DialContext(ctx, "udp", net.JoinHostPort(server, "53"))
			}}
			addrs, err := r.LookupIPAddr(ctx, name)
			ips := []net.IP{}
			for _, a := range addrs {
				ips = append(ips, a.IP)
			}
			return ips, err
		},
		resolvers: func(ctx context.Context) ([]string, []string, error) {
			out, err := exec.CommandContext(ctx, "scutil", "--dns").Output()
			if err != nil {
				return nil, nil, err
			}
			global, all := scutilResolvers(string(out))
			return global, all, nil
		},
	}
}

var scutilNameserver = regexp.MustCompile(`^\s*nameserver\[\d+\]\s*:\s*(\S+)`)

// scutilResolvers reads scutil --dns: the first resolver is the global one;
// the rest come from the supplemental and scoped ones.
func scutilResolvers(out string) (global, all []string) {
	block := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "resolver #") {
			block++
			continue
		}
		m := scutilNameserver.FindStringSubmatch(line)
		if m == nil || net.ParseIP(m[1]) == nil {
			continue
		}
		if block == 1 && !slices.Contains(global, m[1]) {
			global = append(global, m[1])
		}
		if !slices.Contains(all, m[1]) {
			all = append(all, m[1])
		}
	}
	return global, all
}

// privateAddress is an address msb treats as not public: private, shared
// (100.64/10, as VPNs and Tailscale use) or link-local.
func privateAddress(ip net.IP) bool {
	_, shared, _ := net.ParseCIDR("100.64.0.0/10")
	return ip.IsPrivate() || shared.Contains(ip) || ip.IsLinkLocalUnicast()
}

// msbDNSFor works out the resolvers and address rules a sandbox needs for
// the named hosts, and notes for what it cannot do.
func (d *hostDNS) msbDNSFor(ctx context.Context, hosts []string) (nameservers, allow, notes []string) {
	if runtime.GOOS != "darwin" {
		// On Linux msb reads /etc/resolv.conf, whose stub resolver
		// (systemd-resolved) does the split itself.
		return nil, nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var global, all []string
	found := []string{}
	answers := map[string]bool{}
	for _, h := range hosts {
		name := msbHost(h)
		if name == "" || strings.Contains(name, "*") || net.ParseIP(name) != nil || strings.Contains(name, "/") {
			continue
		}
		ips, err := d.lookup(ctx, name)
		if err != nil || len(ips) == 0 {
			notes = append(notes, fmt.Sprintf("%s does not resolve on this machine; connect its VPN and recreate the sandbox to reach it", name))
			continue
		}
		private := []string{}
		for _, ip := range ips {
			if privateAddress(ip) {
				private = append(private, ip.String())
			}
		}
		if len(private) == 0 {
			continue
		}
		if all == nil {
			if global, all, err = d.resolvers(ctx); err != nil {
				notes = append(notes, fmt.Sprintf("cannot read the host's resolvers for %s: %v", name, err))
				return nil, nil, notes
			}
		}
		server := ""
		for _, s := range all {
			got, err := d.query(ctx, s, name)
			if err != nil || !slices.ContainsFunc(got, func(ip net.IP) bool { return slices.Contains(private, ip.String()) }) {
				continue
			}
			if _, ok := answers[s]; !ok {
				public, err := d.query(ctx, s, msbDNSProbe)
				answers[s] = err == nil && len(public) > 0
			}
			if answers[s] {
				server = s
				break
			}
		}
		if server == "" {
			notes = append(notes, fmt.Sprintf("%s has a private address, but no resolver of this machine that gives it answers public names too; msb cannot reach it", name))
			continue
		}
		if !slices.Contains(found, server) {
			found = append(found, server)
		}
		for _, ip := range private {
			if !slices.Contains(allow, ip) {
				allow = append(allow, ip)
			}
		}
		notes = append(notes, fmt.Sprintf("%s resolves through %s, a resolver of this machine msb would not ask; it holds until the sandbox is recreated", name, server))
	}
	if len(found) == 0 {
		return nil, nil, notes
	}
	nameservers = slices.Clone(found)
	for _, s := range global {
		if !slices.Contains(nameservers, s) {
			nameservers = append(nameservers, s)
		}
	}
	return nameservers, allow, notes
}
