package master

import (
	"net"
	"strings"
	"sync"
)

type addressReverse struct {
	addrs map[string]int
	sync.RWMutex
}

func newAddressReverse() *addressReverse {
	return &addressReverse{addrs: make(map[string]int)}
}

func (a *addressReverse) Add(addr net.HardwareAddr, identity int) {
	a.Lock()
	defer a.Unlock()
	a.addrs[strings.ToLower(addr.String())] = identity
}

func (a *addressReverse) Remove(addr net.HardwareAddr) {
	a.Lock()
	defer a.Unlock()
	delete(a.addrs, strings.ToLower(addr.String()))
}

func (a *addressReverse) Get(addr net.HardwareAddr) (identity int, ok bool) {
	identity, ok = a.GetS(addr.String())
	return
}

func (a *addressReverse) GetS(addr string) (identity int, ok bool) {
	a.RLock()
	defer a.RUnlock()
	identity, ok = a.addrs[strings.ToLower(addr)]
	return
}
