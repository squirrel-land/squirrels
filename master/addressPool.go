package master

import (
	"errors"
	"net"
)

var (
	IdentityNotSupported = errors.New("Identity is not managed by this address pool. It needs to be between 1 and Capacity()")
	AddressNotSupported  = errors.New("Address is not in this address pool")
)

type addressPool struct {
	Network *net.IPNet
}

func newAddressPool(network *net.IPNet) (ret *addressPool) {
	return &addressPool{Network: network}
}

func (ap *addressPool) Capacity() int {
	ones, bits := ap.Network.Mask.Size()
	return (1 << uint(bits-ones)) - 2
}

func (ap *addressPool) GetAddress(identity int) (addr net.IP, err error) {
	if identity < 1 || identity > ap.Capacity() {
		err = IdentityNotSupported
		return
	}
	addr = net.IPv4(0, 0, 0, 0).To4()
	copy(addr, ap.Network.IP.To4())
	for i := 1; identity != 0; i++ {
		addr[len(addr)-i] = addr[len(addr)-i] | (byte)(0xFF&identity)
		identity = identity >> 8
	}
	return
}

func (ap *addressPool) GetIdentity(address net.IP) (identity int, err error) {
	if !ap.Network.Contains(address) {
		err = AddressNotSupported
		return
	}
	ones, bits := ap.Network.Mask.Size()
	zeros := 1<<uint(bits-ones) - 1
	identity = 0
	for i := 1; zeros > 0; i++ {
		identity = identity + (0xFF&zeros&int(address[len(address)-i]))<<uint(8*(i-1))
		zeros = zeros >> 8
	}
	return
}

func (ap *addressPool) IsBroadcast(address net.IP) bool {
	ones, bits := ap.Network.Mask.Size()
	zeros := 1<<uint(bits-ones) - 1
	for i := 1; zeros > 0; i++ {
		if 0xFF&zeros&int(address[len(address)-i]) != 0xFF&zeros {
			return false
		}
		zeros = zeros >> 8
	}
	return true
}
