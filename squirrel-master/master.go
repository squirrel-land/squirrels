package main

import (
	"errors"
	"log"
	"net"

	"fmt"
	"github.com/songgao/packets/ethernet"
	"github.com/squirrel-land/squirrel"
	"github.com/squirrel/common"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
	"net/http"
	"time"
)

type client struct {
	Link *common.Link
	Addr net.HardwareAddr
}

type Config struct {
	KeySeed  string
	AuthFile string
	Proxy    string
}

type Master struct {
	Users           common.Users
	fingerprint     string
	addressPool     *addressPool
	clients         []*client
	addrReverse     *addressReverse
	positionManager squirrel.PositionManager

	mobilityManager squirrel.MobilityManager
	september       squirrel.September
	wsServer        websocket.Server
	httpServer      *common.HTTPServer
	sshConfig       *ssh.ServerConfig
	sessions        map[string]*common.User
}

func NewMaster(network *net.IPNet, mobilityManager squirrel.MobilityManager, september squirrel.September, config *Config) (master *Master) {
	master = &Master{
		addressPool:     newAddressPool(network),
		addrReverse:     newAddressReverse(),
		mobilityManager: mobilityManager,
		september:       september,
		wsServer:        websocket.Server{},
		httpServer:      common.NewHTTPServer(),
		sessions:        map[string]*common.User{},
	}

	master.wsServer.Handler = websocket.Handler(master.handleWS)
	//generate private key (optionally using seed)
	key, _ := common.GenerateKey(config.KeySeed)
	//convert into ssh.PrivateKey
	private, err := ssh.ParsePrivateKey(key)
	if err != nil {
		log.Fatal("Failed to parse key")
	}
	//fingerprint this key
	master.fingerprint = common.FingerprintKey(private.PublicKey())
	//create ssh config
	master.sshConfig = &ssh.ServerConfig{
		ServerVersion:    common.ProtocolVersion + "-server",
		PasswordCallback: master.authUser,
	}
	master.sshConfig.AddHostKey(private)

	master.clients = make([]*client, master.addressPool.Capacity()+1, master.addressPool.Capacity()+1)
	master.positionManager = NewPositionManager(master.addressPool.Capacity()+1, master.addrReverse)
	//master.mobilityManager.Initialize(master.positionManager)
	master.september.Initialize(master.positionManager)
	return
}

//
func (master *Master) authUser(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
	// no auth - allow all
	if len(master.Users) == 0 {
		return nil, nil
	}
	// authenticate user
	n := c.User()
	u, ok := master.Users[n]
	if !ok || u.Pass != string(pass) {
		return nil, errors.New("Invalid auth")
	}
	//insert session
	//insert session
	master.sessions[string(c.SessionID())] = u
	return nil, nil
}

func (master *Master) handleWS(ws *websocket.Conn) {
	// Before use, a handshake must be performed on the incoming net.Conn.
	sshConn, chans, reqs, err := ssh.NewServerConn(ws, master.sshConfig)
	if err != nil {
		fmt.Println(err)
	}

	//wait for request, with timeout
	var r *ssh.Request
	select {
	case r = <-reqs:
	case <-time.After(10 * time.Second):
		sshConn.Close()
		return
	}

	r.Reply(true, nil)

	go func() {
		for r := range reqs {
			switch r.Type {
			case "ping":
				r.Reply(true, nil)
			default:
				r.Reply(true, nil)
			}
		}
	}()

	go common.ConnectStreams(chans)
	sshConn.Wait()
	fmt.Printf("Close")
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

func (master *Master) accept(listener net.Listener) (identity int, err error) {
	var connection net.Conn
	connection, err = listener.Accept()
	if err != nil {
		return
	}
	link := common.NewLink(connection)

	var req *common.JoinReq
	req, err = link.GetJoinReq()
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
	return identity, nil
}

func isBroadcast(addr net.HardwareAddr) bool {
	return addr[0] == 0xff && addr[1] == 0xff && addr[2] == 0xff && addr[3] == 0xff && addr[4] == 0xff && addr[5] == 0xff
}

func isIPv4Multicast(addr net.HardwareAddr) bool {
	return addr[0] == 0x01 && addr[1] == 0x00 && addr[2] == 0x5e
}

func (master *Master) frameHandler(myIdentity int) {
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
					if *debug {
						log.Printf("broadcast frame of length %d from client %d to be delivered to client %d\n", len(frame.Payload()), myIdentity, id)
					}
				}
			}
			buf.Done()
		} else { // unicast
			dstID, ok := master.addrReverse.Get(dst)
			if ok {
				if master.september.SendUnicast(myIdentity, dstID, len(frame.Payload())) {
					master.clients[dstID].Link.WriteFrame(buf)
					if *debug {
						log.Printf("unicast frame of length %d from client %d to be delivered to client %d\n", len(frame.Payload()), myIdentity, dstID)
					}
				} else {
					buf.Done()
					if *debug {
						log.Printf("unicast frame of length %d from client %d NOT to be delivered to client %d\n", len(frame.Payload()), myIdentity, dstID)
					}
				}
			} else {
				if *debug {
					log.Printf("unicast frame of length %d from client %d has unknown dst address: %v\n", len(frame.Payload()), myIdentity, dst)
				}
			}
		}
	}
	master.clientLeave(myIdentity, master.clients[myIdentity].Link.IncomingError())
}

func (master *Master) handleHTTP(w http.ResponseWriter, r *http.Request) {
	//websockets upgrade AND has chisel prefix
	if r.Header.Get("Upgrade") == "websocket" &&
		r.Header.Get("Sec-WebSocket-Protocol") == common.ProtocolVersion {
		master.wsServer.ServeHTTP(w, r)
		return
	}
	//missing :O
	w.WriteHeader(404)
}

func (master *Master) Run(laddr string) (err error) {
	var (
		identity int
		listener net.Listener
	)

	listener, err = net.Listen("tcp", laddr)

	//listener, err = master.httpServer.GoListenAndServe(laddr, http.HandlerFunc(master.handleHTTP))

	if err != nil {
		return
	}
	for {
		identity, err = master.accept(listener)
		if err != nil {
			continue
		}
		go master.frameHandler(identity)
	}
	return
}
