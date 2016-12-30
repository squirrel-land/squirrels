package master

import (
	"errors"
	"log"
	"net"

	"github.com/overlay/common"
	"github.com/songgao/packets/ethernet"
	"golang.org/x/net/websocket"
	"net/http"
	"sync"
)

type client struct {
	Link *common.Link
	Addr net.HardwareAddr
}

type Master struct {
	*common.Logger
	addressPool     *addressPool
	clients         []*client
	addrReverse     *addressReverse
	positionManager common.PositionManager
	september       common.September
	wsServer        websocket.Server
	httpServer      *common.HTTPServer
}

func NewMaster(network *net.IPNet, september common.September) (master *Master) {
	master = &Master{
		Logger:      common.NewLogger("server"),
		addressPool: newAddressPool(network),
		addrReverse: newAddressReverse(),
		september:   september,
		wsServer:    websocket.Server{},
		httpServer:  common.NewHTTPServer(),
	}
	master.wsServer.Handler = websocket.Handler(master.handleWS)
	master.clients = make([]*client, master.addressPool.Capacity()+1, master.addressPool.Capacity()+1)
	master.positionManager = NewNodeManager(master.addressPool.Capacity()+1, master.addrReverse)
	master.september.Initialize(master.positionManager)
	return master
}

func (master *Master) clientJoin(identity int, addr net.HardwareAddr, link *common.Link) {
	master.clients[identity] = &client{Link: link, Addr: addr}
	master.positionManager.Enable(identity)
	master.addrReverse.Add(addr, identity)
	ipAddr, _ := master.addressPool.GetAddress(identity)
	log.Printf("%v joined\n", ipAddr)
}

func (master *Master) clientLeave(identity int, err error) {
	master.addrReverse.Remove(master.clients[identity].Addr)
	master.clients[identity] = nil
	master.positionManager.Disable(identity)
	addr, _ := master.addressPool.GetAddress(identity)
	if err == nil {
		log.Printf("link to %v is terminated with no error\n", addr)
	} else {
		log.Printf("link to %v is terminated with error: %v\n", addr, err)
	}
	log.Printf("%v left\n", addr)
}

func isBroadcast(addr net.HardwareAddr) bool {
	return addr[0] == 0xff && addr[1] == 0xff && addr[2] == 0xff && addr[3] == 0xff && addr[4] == 0xff && addr[5] == 0xff
}

func isIPv4Multicast(addr net.HardwareAddr) bool {
	return addr[0] == 0x01 && addr[1] == 0x00 && addr[2] == 0x5e
}

func (master *Master) frameHandler(myIdentity int, wg *sync.WaitGroup) {
	var (
		buf        *common.ReusableSlice
		ok         bool
		underlying = make([]int, master.addressPool.Capacity()+1)
	)

	for {
		buf, ok = master.clients[myIdentity].Link.ReadFrame()
		if !ok {
			break
		}
		frame := ethernet.Frame(buf.Slice())
		dst := frame.Destination()
		if isBroadcast(dst) || isIPv4Multicast(dst) {
			recipients := master.september.SendBroadcast(myIdentity, len(frame.Payload()), underlying)
			for _, id := range recipients {
				if master.clients[id] != nil {
					buf.AddOwner()
					master.clients[id].Link.WriteFrame(buf)
					//if *debug {
					log.Printf("broadcast frame of length %d from client %d to be delivered to client %d\n", len(frame.Payload()), myIdentity, id)
					//}

				}
			}
			buf.Done()
		} else {
			// unicast
			dstID, ok := master.addrReverse.Get(dst)
			if ok {
				if master.september.SendUnicast(myIdentity, dstID, len(frame.Payload())) {
					master.clients[dstID].Link.WriteFrame(buf)
					//if *debug {
					log.Printf("unicast frame of length %d from client %d to be delivered to client %d\n", len(frame.Payload()), myIdentity, dstID)
					//}
				} else {
					buf.Done()
					//if *debug {
					log.Printf("unicast frame of length %d from client %d NOT to be delivered to client %d\n", len(frame.Payload()), myIdentity, dstID)
					//}
				}
			} else {
				//if *debug {
				log.Printf("unicast frame of length %d from client %d has unknown dst address: %v\n", len(frame.Payload()), myIdentity, dst)
				//}
			}
		}
	}
	master.clientLeave(myIdentity, master.clients[myIdentity].Link.IncomingError())
	wg.Done()
}

func (m *Master) Run(host, port string) error {
	if err := m.Start(host, port); err != nil {
		return err
	}
	return m.Wait()
}

func (m *Master) Start(host, port string) error {
	m.Infof("Listening on %s...", port)
	return m.httpServer.GoListenAndServe(host+":"+port, http.HandlerFunc(m.handleHTTP))
}

func (m *Master) Wait() error {
	return m.httpServer.Wait()
}

func (m *Master) Close() error {
	//this should cause an error in the open websockets
	return m.httpServer.Close()
}

func (master *Master) handleHTTP(w http.ResponseWriter, r *http.Request) {
	//websockets upgrade AND has chisel prefix
	if r.Header.Get("Upgrade") == "websocket" {
		master.wsServer.ServeHTTP(w, r)
		return
	}
	//missing :O
	w.WriteHeader(404)
}

func (master *Master) handleWS(ws *websocket.Conn) {
	var (
		identity int
	)
	link := common.NewLink(ws)

	var req *common.JoinReq
	req, err := link.GetJoinReq()
	if err != nil {
		return
	}

	for identity = 1; identity < len(master.clients); identity++ {
		if master.clients[identity] == nil {
			break
		}
	}
	if identity == len(master.clients) {
		err = errors.New("Adress poll is full")
		link.SendJoinRsp(&common.JoinRsp{Error: err})
		return
	}

	var addr net.IP
	addr, err = master.addressPool.GetAddress(identity)
	if err != nil {
		return
	}
	err = link.SendJoinRsp(&common.JoinRsp{Address: addr, Mask: master.addressPool.Network.Mask, Error: nil})
	if err != nil {
		return
	}
	master.clientJoin(identity, req.MACAddr, link)
	link.StartRoutines()

	var wg sync.WaitGroup
	wg.Add(1)
	go master.frameHandler(identity, &wg)
	wg.Wait()
}
