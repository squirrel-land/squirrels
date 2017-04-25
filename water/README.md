# water

`water` is a native Go library for [TUN/TAP](http://en.wikipedia.org/wiki/TUN/TAP) interfaces.

`water` is designed to be simple and efficient. It

* wraps almost only syscalls and uses only Go standard types;
* exposes standard interfaces; plays well with standard packages like `io`, `bufio`, etc..
* does not handle memory management (allocating/destructing slice). It's up to user to decide whether/how to reuse buffers.

~~`water/waterutil` has some useful functions to interpret MAC frame headers and IP packet headers. It also contains some constants such as protocol numbers and ethernet frame types.~~

See https://github.com/songgao/packets for functions for parsing various packets.

## Supported Platforms

* Linux
* macOS (point-to-point TUN only)

## Installation
```
go get -u github.com/songgao/water
go get -u github.com/songgao/water/waterutil
```

## Documentation
[http://godoc.org/github.com/songgao/water](http://godoc.org/github.com/songgao/water)

## Example

### TAP on Linux:

```go
package main

import (
	"log"

	"github.com/songgao/packets/ethernet"
	"github.com/songgao/water"
)

func main() {
	ifce, err := water.NewTAP("O_O")
	if err != nil {
		log.Fatal(err)
	}
	var frame ethernet.Frame

	for {
		frame.Resize(1500)
		n, err := ifce.Read([]byte(frame))
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
```

This piece of code creates a `TAP` interface, and prints some header information for every frame. After pull up the `main.go`, you'll need to bring up the interface and assign an IP address. All of these need root permission.

```bash
sudo go run main.go
```

In a new terminal:

```bash
sudo ip addr add 10.1.0.10/24 dev O_O
sudo ip link set dev O_O up
```

Wait until the output `main.go` terminal, try sending some ICMP broadcast message:
```bash
ping -c1 -b 10.1.0.255
```

You'll see output containing the IPv4 ICMP frame:
```
2016/10/24 03:18:16 Dst: ff:ff:ff:ff:ff:ff
2016/10/24 03:18:16 Src: 72:3c:fc:29:1c:6f
2016/10/24 03:18:16 Ethertype: 08 00
2016/10/24 03:18:16 Payload: 45 00 00 54 00 00 40 00 40 01 25 9f 0a 01 00 0a 0a 01 00 ff 08 00 01 c1 08 49 00 01 78 7d 0d 58 00 00 00 00 a2 4c 07 00 00 00 00 00 10 11 12 13 14 15 16 17 18 19 1a 1b 1c 1d 1e 1f 20 21 22 23 24 25 26 27 28 29 2a 2b 2c 2d 2e 2f 30 31 32 33 34 35 36 37
```

### TUN on macOS

```go
package main

import (
	"log"

	"github.com/songgao/water"
)

func main() {
	ifce, err := water.NewTUN("")
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Interface Name: %s\n", ifce.Name())

	packet := make([]byte, 2000)
	for {
		n, err := ifce.Read(packet)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Packet Received: % x\n", packet[:n])
	}
}
```

Run it!

```bash
$ sudo go run main.go
```

This is a point-to-point only interface. Use `ifconfig` to see its attributes. You need to bring it up and assign IP addresses (apparently replace `utun2` if needed):

```bash
$ sudo ifconfig utun2 10.1.0.10 10.1.0.20 up
```

Now send some ICMP packets to the interface:

```bash
$ ping 10.1.0.20
```

You'd see the ICMP packets printed out:

```
2016/10/23 20:21:53 Interface Name: utun2
2016/10/23 20:22:40 Packet Received: 00 00 00 02 45 00 00 54 4a 2e 00 00 40 01 1c 5c 0a 01 00 0a 0a 01 00 14 08 00 31 51 f0 f9 00 00 58 0d 7e 80 00 03 14 21 08 09 0a 0b 0c 0d 0e 0f 10 11 12 13 14 15 16 17 18 19 1a 1b 1c 1d 1e 1f 20 21 22 23 24 25 26 27 28 29 2a 2b 2c 2d 2e 2f 30 31 32 33 34 35 36 37
```

## TODO
* tuntaposx for TAP on Darwin

## Alternatives
`tuntap`: [https://code.google.com/p/tuntap/](https://code.google.com/p/tuntap/)
