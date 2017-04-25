package common

import "github.com/coreos/go-etcd/etcd"

// September is used by Master to make decisions upon packets.
//
// Interference, Packet Loss, etc. should be modeled in an implementation of
// this interface.
//
// Name September is from TV series Fringe http://fringe.wikia.com/wiki/September
// for his supernatural ability to observe everything and to manipulate (nearly) every object.
type September interface {
	// Configure configures the node manager with a set of parameters.
	Configure(*etcd.Node) error

	// SendUnicast is used when a unicast packet as large as size(in bytes) is
	// sent from source(identity) to destination(identity).
	//
	// Returns whether the packet should be delivered.
	//
	// Any modification to models (interference, etc.) should be done within this
	// function.
	SendUnicast(source int, destination int, size int) bool

	// SendBroadcast is used when a broadcast packet as large as size(in bytes)
	// is sent from source(identity).
	//
	// Returns a slice of non-nil identities of nodes that should receive this
	// packet. For effiency, the returned slice is a sub-slice of underlying.
	//
	// underlying is a slice that garantees length large enough to hold all
	// nodes. It does nothing more than providing a dedicated space for returned
	// identities from this method. It's intended for reducing workload of GC.
	// Thus, this method should modify elements in underlying and the returned
	// slice should be a sub-slice of underlying.
	//
	// Any modification to models (interference, etc.) should be done within this
	// function.
	SendBroadcast(source int, size int, underlying []int) []int
	Initialize(positionManager PositionManager)
}

type PositionManager interface {

	// Enable marks a node as enabled.
	Enable(index int)

	// Disable marks a node as disabled.
	Disable(index int)

	// IsEnabled returns whether a node is enabled.
	IsEnabled(index int) bool

	Enabled() []int

	// RegisterEnabledChanged registers a channel, which when a node is enabled
	// or disabled, is used to send a slice of indices of all enabled nodes.
	RegisterEnabledChanged(channel chan<- []int)
}
