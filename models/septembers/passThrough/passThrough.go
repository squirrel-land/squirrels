package passThrough

import (
	"github.com/coreos/go-etcd/etcd"
	"github.com/overlay/common"
)

var Septembers = map[string]func() common.September{
	"PassThrough": CreateSeptember,
	/* legacy names */
	"September0th": CreateSeptember,
}

type passThrough struct {
	positionManager common.PositionManager
}

func CreateSeptember() common.September {
	return &passThrough{}
}

func (p *passThrough) ParametersHelp() string {
	return `PassThrough delivers every packet sent into overlay as long as the src and dst are valid.`
}

func (p *passThrough) Configure(conf *etcd.Node) (err error) {
	return nil
}

func (p *passThrough) Initialize(positionManager common.PositionManager) {
	p.positionManager = positionManager
}

func (p *passThrough) SendUnicast(source int, destination int, size int) bool {
	return p.isToBeDelivered(source, destination)
}

func (p *passThrough) SendBroadcast(source int, size int, underlying []int) []int {
	count := 0
	for _, i := range p.positionManager.Enabled() {
		if i != source {
			underlying[count] = i
			count++
		}
	}
	return underlying[:count]
}

func (p *passThrough) isToBeDelivered(id1 int, id2 int) bool {
	if p.positionManager.IsEnabled(id1) && p.positionManager.IsEnabled(id2) {
		return true
	} else {
		return false
	}
}
