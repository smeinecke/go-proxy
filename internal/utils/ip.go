package utils

import (
	"net"
	"net/netip"
)

// GenerateNetIP generates a random netip.Addr within the given prefix.
func GenerateNetIP(prefix netip.Prefix) (netip.Addr, error) {
	if prefix.IsSingleIP() {
		return prefix.Addr(), nil
	}

	bits := prefix.Bits()
	hostBits := prefix.Addr().BitLen() - bits

	if prefix.Addr().Is4() {
		base := prefix.Addr().As4()
		result := make([]byte, 4)
		copy(result, base[:])
		for i := 0; i < hostBits/8; i++ {
			idx := len(result) - 1 - i
			if idx >= 0 {
				result[idx] = byte(RandomInt(256))
			}
		}
		// Handle remaining bits
		remaining := hostBits % 8
		if remaining > 0 {
			mask := byte(0xFF >> remaining)
			result[3-hostBits/8] |= byte(RandomInt(1 << remaining))
			result[3-hostBits/8] &= ^mask
		}
		return netip.AddrFrom4([4]byte(result)), nil
	}

	// IPv6
	base := prefix.Addr().As16()
	result := make([]byte, 16)
	copy(result, base[:])
	for i := 0; i < hostBits/8; i++ {
		idx := len(result) - 1 - i
		if idx >= 0 {
			result[idx] = byte(RandomInt(256))
		}
	}
	remaining := hostBits % 8
	if remaining > 0 {
		mask := byte(0xFF >> remaining)
		result[15-hostBits/8] |= byte(RandomInt(1 << remaining))
		result[15-hostBits/8] &= ^mask
	}
	return netip.AddrFrom16([16]byte(result)), nil
}

// GenerateIP generates a random IP address based on the given CIDR
func GenerateIP(cidr net.IPNet) (net.IP, error) {
	if cidr.IP.To4() != nil {
		return generateIPv4(cidr.IP, cidr.Mask), nil
	}

	return generateIPv6(cidr.IP, cidr.Mask), nil
}

// generateIPv4 generates a random IPv4 address within the given network
func generateIPv4(ip net.IP, mask net.IPMask) net.IP {
	// Convert to 4-byte representation
	ip = ip.To4()
	if ip == nil {
		return nil
	}

	// Create a new IP with the same network portion
	result := make(net.IP, len(ip))
	copy(result, ip)

	// Calculate the number of host bits
	ones, _ := mask.Size()
	hostBits := 32 - ones

	// Generate random bits only for the host portion
	for i := 0; i < hostBits/8; i++ {
		byteIndex := len(result) - 1 - i
		if byteIndex >= 0 {
			result[byteIndex] = byte(RandomInt(256))
		}
	}

	return result
}

// generateIPv6 generates a random IPv6 address within the given network
func generateIPv6(ip net.IP, mask net.IPMask) net.IP {
	// Create a new IP with the same network portion
	result := make(net.IP, len(ip))
	copy(result, ip)

	// Calculate the number of host bits
	ones, _ := mask.Size()
	hostBits := 128 - ones

	// Generate random bits only for the host portion
	for i := 0; i < hostBits/8; i++ {
		byteIndex := len(result) - 1 - i
		if byteIndex >= 0 {
			result[byteIndex] = byte(RandomInt(256))
		}
	}

	return result
}
