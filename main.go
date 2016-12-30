package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/coreos/go-etcd/etcd"
	"github.com/overlay/common"
	"github.com/overlay/master"
	"github.com/overlay/worker"
	"log"
	"net"
	"os"
)

var VERSION string = "0.0.0-src" //set via ldflags

var help = `
	Usage: overlay [command] [--help]

	Version: ` + VERSION + `

	Commands:
	  server - runs in server mode
	  client - runs in client mode

	Read more:
	  https://github.com/kenan435/overlay

`

func main() {
	version := flag.Bool("version", false, "")
	flag.Bool("help", false, "")
	flag.Bool("h", false, "")
	flag.Usage = func() {}
	flag.Parse()

	if *version {
		fmt.Println(VERSION)
		os.Exit(1)
	}

	args := flag.Args()

	subcmd := ""
	if len(args) > 0 {
		subcmd = args[0]
		args = args[1:]
	}

	switch subcmd {
	case "server":
		server(args)
	case "client":
		client(args)
	default:
		fmt.Fprintf(os.Stderr, help)
		os.Exit(1)
	}
}

var commonHelp = `
	  -v, Enable verbose logging

	  --help, This help text

	Read more:
	  https://github.com/kenan435/overlay

`

var serverHelp = `
	Usage: chisel server [options]

	Options:

	  --host, Defines the HTTP listening host – the network interface
	  (defaults to 0.0.0.0).

	  --port, Defines the HTTP listening port (defaults to 8080).

` + commonHelp

func server(args []string) {

	flags := flag.NewFlagSet("server", flag.ContinueOnError)

	host := flags.String("host", "", "")
	port := flags.String("port", "", "")
	verbose := flags.Bool("v", false, "")

	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, serverHelp)
		os.Exit(1)
	}
	flags.Parse(args)

	if *host == "" {
		*host = os.Getenv("HOST")
	}
	if *host == "" {
		*host = "0.0.0.0"
	}

	if *port == "" {
		*port = os.Getenv("PORT")
	}
	if *port == "" {
		*port = "8080"
	}

	conf, err := getConfig()
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	var network *net.IPNet
	_, network, err = net.ParseCIDR(conf.emulatedSubnet)
	if err != nil {
		return
	}
	var september common.September
	september, err = master.NewSeptember(conf.september)
	if err != nil {
		return
	}

	err = september.Configure(conf.septemberConfig)
	if err != nil {
		log.Println("Creating September failed. Following message might help:\n")
		return
	}

	m := master.NewMaster(network, september)
	if err != nil {
		log.Fatal(err)
	}

	m.Info = true
	m.Debug = *verbose

	if err = m.Run(*host, *port); err != nil {
		log.Fatal(err)
	}
}

var clientHelp = `
	Usage: chisel client [options] <server> <remote> [remote] [remote] ...

	server is the URL to the chisel server.

	remotes are remote connections tunnelled through the server, each of
	which come in the form:

		<local-host>:<local-port>:<remote-host>:<remote-port>

		* remote-port is required.
		* local-port defaults to remote-port.
		* local-host defaults to 0.0.0.0 (all interfaces).
		* remote-host defaults to 0.0.0.0 (server localhost).

		example remotes

			3000
			example.com:3000
			3000:google.com:80
			192.168.0.5:3000:google.com:80

	Options:

	  --fingerprint, An optional fingerprint (server authentication)
	  string to compare against the server's public key. You may provide
	  just a prefix of the key or the entire string. Fingerprint
	  mismatches will close the connection.

	  --auth, An optional username and password (client authentication)
	  in the form: "<user>:<pass>". These credentials are compared to
	  the credentials inside the server's --authfile.

	  --keepalive, An optional keepalive interval. Since the underlying
	  transport is HTTP, in many instances we'll be traversing through
	  proxies, often these proxies will close idle connections. You must
	  specify a time with a unit, for example '30s' or '2m'. Defaults
	  to '0s' (disabled).
` + commonHelp

func client(args []string) {

	var (
		clientConf clientConfig
		err        error
	)

	flags := flag.NewFlagSet("client", flag.ContinueOnError)

	verbose := flags.Bool("v", false, "")
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, clientHelp)
		os.Exit(1)
	}
	flags.Parse(args)
	//pull out options, put back remaining args
	args = flags.Args()
	if len(args) < 1 {
		log.Fatalf("A server is required")
	}

	if clientConf, err = getClientConfig(); err != nil {

		log.Fatalf("reading config error: %v\n", err)
	}

	c, err := worker.NewClient(clientConf.tapName, &worker.Config{
		Server: args[0],
	})
	if err != nil {
		log.Fatal(err)
	}

	c.Info = true
	c.Debug = *verbose

	if err = c.Run(); err != nil {
		log.Fatal(err)
	}
}

func getClientConfig() (clientConfig clientConfig, err error) {
	endpoint := os.Getenv("SQUIRREL_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://172.31.0.209:2379"
	}
	client := etcd.NewClient([]string{endpoint})

	clientConfig.masterURI, err = common.GetEtcdValue(client, "/overlay/master_uri")
	if err != nil {
		return
	}

	clientConfig.tapName, err = common.GetEtcdValue(client, "/overlay/worker_tap_name")
	if err != nil {
		if common.IsEtcdNotFoundError(err) {
			// syscalls in `water` uses default TAP interface name if empty
			err = nil
		} else {
			return
		}
	}

	return
}

type clientConfig struct {
	masterURI string
	tapName   string
}

type config struct {
	uri               string
	emulatedSubnet    string
	nodeManager       string
	nodeManagerConfig *etcd.Node
	september         string
	septemberConfig   *etcd.Node
}

// get ifce that is up and has a external ip assigned
func ifce() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}

		return iface.Name, nil
	}
	return "", errors.New("are you connected to the network?")
}

//get ip addr of interface, make sure only one ip is assinged to ifce
func getAddr(interfaceName string) (net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, ifce := range interfaces {
		if ifce.Name == interfaceName {
			addrs, err := ifce.Addrs()
			if err != nil {
				return nil, err
			}
			var ipAddrs []net.IP
			for _, addr := range addrs {
				ipNet, ok := addr.(*net.IPNet)
				if ok {
					ip4 := ipNet.IP.To4()
					if ip4 != nil {
						ipAddrs = append(ipAddrs, ip4)
					}
				}
			}

			if len(ipAddrs) != 1 {
				return nil, fmt.Errorf("Configured inteface (%s) has wrong number of IP addresses. Expected %d, got %d", ifce.Name, 1, len(ipAddrs))
			}
			return ipAddrs[0], nil
		}
	}
	return nil, fmt.Errorf("Configured interface (%s) is not found", interfaceName)
}

func getConfig() (conf config, err error) {
	endpoint := os.Getenv("SQUIRREL_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:2379"
	}
	client := etcd.NewClient([]string{endpoint})

	ifce, err := ifce()
	if err != nil {
		return
	}

	var addr net.IP
	addr, err = getAddr(ifce)
	if err != nil {
		return
	}
	conf.uri = addr.String() + ":1234"

	_, err = client.Set("/overlay/master_ip", addr.String(), 0)
	if err != nil {
		return
	}
	_, err = client.Set("/overlay/master_uri", conf.uri, 0)
	if err != nil {
		return
	}

	_, err = client.Set("/overlay/master/emulated_subnet", "10.0.4.0/24", 0)
	conf.emulatedSubnet, err = common.GetEtcdValue(client, "/overlay/master/emulated_subnet")
	if err != nil {
		return
	}

	conf.emulatedSubnet, err = common.GetEtcdValue(client, "/overlay/master/emulated_subnet")
	if err != nil {
		return
	}

	var nodeManagerConfigPath string
	nodeManagerConfigPath, err = common.GetEtcdValue(client, "/overlay/master/mobility_manager_config_path")
	if err != nil {
		if common.IsEtcdNotFoundError(err) {
			err = nil
		} else {
			return
		}
	} else {
		var resp *etcd.Response
		resp, err = client.Get(nodeManagerConfigPath, false, true)
		if err != nil {
			return
		}
		if !resp.Node.Dir {
			err = errors.New("nodeManagerConfig is not a Dir node")
			return
		}
		conf.nodeManagerConfig = resp.Node
	}

	_, err = client.Set("/overlay/master/september", "September0th", 0)
	conf.september, err = common.GetEtcdValue(client, "/overlay/master/september")
	if err != nil {
		return
	}

	var septemberConfigPath string
	septemberConfigPath, err = common.GetEtcdValue(client, "/overlay/master/september_config_path")
	if err != nil {
		if common.IsEtcdNotFoundError(err) {
			err = nil
		} else {
			return
		}
	} else {
		var resp *etcd.Response
		resp, err = client.Get(septemberConfigPath, false, true)
		if err != nil {
			return
		}
		if !resp.Node.Dir {
			err = errors.New("septemberConfig is not a Dir node")
			return
		}
		conf.septemberConfig = resp.Node
	}

	return
}
