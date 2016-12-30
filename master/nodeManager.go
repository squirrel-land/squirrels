package master

import (
	"github.com/overlay/common"
	"sync"
)

type PositionManager struct {
	mu []*sync.RWMutex

	isEnabled      []bool
	enabledChanged []chan<- []int
	muEnabled      *sync.RWMutex // mutex for isEnabled, enabled and enabledChanged

	addrReverse *addressReverse
}

func NewNodeManager(size int, addrReverse *addressReverse) common.PositionManager {
	ret := new(PositionManager)
	ret.mu = make([]*sync.RWMutex, size)
	ret.isEnabled = make([]bool, size)
	ret.enabledChanged = make([]chan<- []int, 0)
	ret.muEnabled = new(sync.RWMutex)
	ret.addrReverse = addrReverse
	for i := 0; i < size; i++ {
		ret.mu[i] = new(sync.RWMutex)
	}
	return ret
}

// Enable marks a node enabled.
func (p *PositionManager) Enable(index int) {
	p.muEnabled.Lock()
	defer p.muEnabled.Unlock()
	p.isEnabled[index] = true
	p.notifyEnabledChanged()
}

// Disable marks a node disabled.
func (p *PositionManager) Disable(index int) {
	p.muEnabled.Lock()
	defer p.muEnabled.Unlock()
	p.isEnabled[index] = false
	p.notifyEnabledChanged()
}

func (p *PositionManager) IsEnabled(index int) bool {
	p.muEnabled.RLock()
	defer p.muEnabled.RUnlock()
	return p.isEnabled[index]
}

func (p *PositionManager) calculateEnabled() []int {
	e := make([]int, 0)
	for i, v := range p.isEnabled {
		if v {
			e = append(e, i)
		}
	}
	return e
}

func (p *PositionManager) Enabled() []int {
	p.muEnabled.RLock()
	defer p.muEnabled.RUnlock()
	return p.calculateEnabled()
}

// RegisterEnabledChanged registers a channel used to receive a slice of
// indices of all enabled nodes.  Slice is sent into channel each time any node
// is enabled/disabled.
func (p *PositionManager) RegisterEnabledChanged(channel chan<- []int) {
	p.muEnabled.Lock()
	defer p.muEnabled.Unlock()
	p.enabledChanged = append(p.enabledChanged, channel)
}

func (p *PositionManager) notifyEnabledChanged() {
	for _, c := range p.enabledChanged {
		c <- p.calculateEnabled()
	}
}
