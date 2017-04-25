package worker

import (
	"fmt"
	"github.com/overlay/common"
	"github.com/overlay/packets/ethernet"
	"github.com/overlay/water"
	"golang.org/x/net/websocket"
	"log"
	"net"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
)

type Config struct {
	Server string
}

type Client struct {
	server string
	*common.Logger
	link     *common.Link
	tap      *water.Interface
	running  bool
	runningc chan error
}

// Create a new client along with a TAP network interface whose name is tapName
func NewClient(tapName string, config *Config) (client *Client, err error) {
	//apply default scheme
	if !strings.HasPrefix(config.Server, "http") {
		config.Server = "http://" + config.Server
	}

	u, err := url.Parse(config.Server)
	if err != nil {
		return nil, err
	}

	//apply default port
	if !regexp.MustCompile(`:\d+$`).MatchString(u.Host) {
		if u.Scheme == "https" || u.Scheme == "wss" {
			u.Host = u.Host + ":443"
		} else {
			u.Host = u.Host + ":8080"
		}
	}

	//swap to websockets scheme
	u.Scheme = strings.Replace(u.Scheme, "http", "ws", 1)
	var tap *water.Interface
	tap, err = water.NewTAP(tapName)
	if err != nil {
		return nil, err
	}
	client = &Client{
		Logger:   common.NewLogger("client"),
		server:   u.String(),
		link:     nil,
		tap:      tap,
		running:  true,
		runningc: make(chan error, 1),
	}
	return
}

//Start then Wait
func (c *Client) Run() error {
	go c.start()
	return c.Wait()
}

//Wait blocks while the client is running
func (c *Client) Wait() error {
	return <-c.runningc
}

func (client *Client) printFrame() {
	var frame ethernet.Frame

	for {
		frame.Resize(1500)
		n, err := client.tap.Read([]byte(frame))
		if err != nil {
			log.Fatal(err)
		}
		frame = frame[:n]
		log.Printf("Dst: %s\n", frame.Destination())
		log.Printf("Src: %s\n", frame.Source())
		log.Printf("Ethertype: % x\n", frame.Ethertype())
		log.Printf("Payload: % x\n", frame.Payload())
	}
}

func (client *Client) configureTap(joinRsp *common.JoinRsp) (err error) {
	m, _ := joinRsp.Mask.Size()
	addr := fmt.Sprintf("%s/%d", joinRsp.Address.String(), m)
	log.Printf("Assigning %s to %s\n", addr, client.tap.Name())
	err = exec.Command("ip", "addr", "add", addr, "dev", client.tap.Name()).Run()
	if err != nil {
		return
	}
	err = exec.Command("ip", "link", "set", "dev", client.tap.Name(), "up").Run()
	return
}

func (client *Client) connect(masterAddr string) (err error) {
	client.Infof("Connecting to %s\n", client.server)
	connection, err := websocket.Dial(masterAddr, "", "http://localhost/")

	client.link = common.NewLink(connection)
	var ifce *net.Interface
	ifce, err = net.InterfaceByName(client.tap.Name())
	err = client.link.SendJoinReq(&common.JoinReq{MACAddr: ifce.HardwareAddr})
	if err != nil {
		client.Infof("Sending joining request failed")
		client.Debugf(err.Error())
	}
	var rsp *common.JoinRsp
	rsp, err = client.link.GetJoinRsp()
	if rsp.Error != nil {
		client.Infof("Join failed")
		client.Debugf(rsp.Error.Error())
	}
	err = client.configureTap(rsp)
	if err != nil {
		client.Infof("Configure Tap failed")
		client.Debugf(err.Error())
	}
	client.link.StartRoutines()
	return
}

func (client *Client) tap2master() {
	var err error
	pool := common.NewSlicePool(1522)
	var n int
	for {
		buf := pool.Get()
		if n, err = client.tap.Read(buf.Slice()); err != nil {
			log.Fatalf("reading from tap error: %v\n", err)
			return
		}
		buf.Resize(n)
		client.link.WriteFrame(buf)
	}
}

func (client *Client) master2tap() {
	var (
		buf *common.ReusableSlice
		err error
		ok  bool
	)
	for {
		buf, ok = client.link.ReadFrame()
		if !ok {
			break
		}
		_, err = client.tap.Write(buf.Slice())
		buf.Done()
		if err != nil {
			log.Fatalf("writing to TAP error: %v\n", err)
			return
		}
	}
	if client.link.IncomingError() == nil {
		log.Println("link terminated with no error")
	} else {
		log.Fatalf("link terminated with error: %v\n", client.link.IncomingError())
	}
}

// Run the client, and block until all routines exit or any error is ecountered.
// It connects to a master with address masterAddr, proceeds with JoinReq/JoinRsp process, configures the TAP device, and at last, start routines that carry MAC frames back and forth between the TAP device and the master.
// masterAddr: should be host:port format where host can be either IP address or hostname/domainName.
func (c *Client) start() (err error) {
	err = c.connect(c.server)
	if err != nil {
		return
	}

	go c.printFrame()

	go c.tap2master()
	go c.master2tap()

	return
}
