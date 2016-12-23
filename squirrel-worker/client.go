package main

import (
	"fmt"
	"log"
	"net"
	"os/exec"

	"github.com/songgao/packets/ethernet"
	"github.com/squirrel-land/squirrel/common"
	"github.com/squirrel-land/water"
)

type Client struct {
	link *common.Link
	tap  *water.Interface
}

// Create a new client along with a TAP network interface whose name is tapName
func NewClient(tapName string) (client *Client, err error) {
	var tap *water.Interface
	tap, err = water.NewTAP(tapName)
	if err != nil {
		return nil, err
	}
	client = &Client{
		link: nil,
		tap:  tap,
	}
	return
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
	var connection net.Conn
	connection, err = net.Dial("tcp", masterAddr)
	if err != nil {
		return
	}
	client.link = common.NewLink(connection)
	if err != nil {
		return
	}

	client.link = common.NewLink(connection)

	var ifce *net.Interface
	ifce, err = net.InterfaceByName(client.tap.Name())
	err = client.link.SendJoinReq(&common.JoinReq{MACAddr: ifce.HardwareAddr})
	if err != nil {
		return
	}
	var rsp *common.JoinRsp
	rsp, err = client.link.GetJoinRsp()
	if err != nil {
		return
	}
	if rsp.Error != nil {
		return fmt.Errorf("Join failed: %s", rsp.Error.Error())
	}
	err = client.configureTap(rsp)
	if err != nil {
		return
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
func (client *Client) Start(masterAddr string) (err error) {
	err = client.connect(masterAddr)
	if err != nil {
		return
	}

	go client.printFrame()
	go client.tap2master()
	go client.master2tap()

	return
}
