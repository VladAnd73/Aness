package isolate

import "bytes"

// HostsFile is the host's hosts file plus a sink line per blocked host
// in both families: getaddrinfo stops at a files match, but an IPv6-only
// lookup with no IPv6 line falls through to real DNS.
func HostsFile(base []byte) []byte {
	var b bytes.Buffer
	b.Write(base)
	if len(base) > 0 && base[len(base)-1] != '\n' {
		b.WriteByte('\n')
	}
	for _, h := range BlockedHosts {
		b.WriteString("0.0.0.0 " + h + "\n")
		b.WriteString(":: " + h + "\n")
	}
	return b.Bytes()
}
