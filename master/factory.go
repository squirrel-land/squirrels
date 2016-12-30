package master

import (
	"errors"

	"github.com/overlay/common"
	"github.com/overlay/models/septembers/passThrough"
)

var (
	notRegistered = errors.New("MobilityManager or September is not registered.")
)

func NewSeptember(name string) (september common.September, err error) {
	constructor := passThrough.Septembers[name]
	if constructor == nil {
		return nil, notRegistered
	}
	september = constructor()
	return
}
