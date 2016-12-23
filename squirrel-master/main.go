package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"runtime/pprof"
	"time"

	"github.com/coreos/go-etcd/etcd"
	_ "github.com/songgao/stacktraces/on/SIGUSR1"
	"github.com/squirrel-land/squirrel"
	"github.com/squirrel-land/squirrel/common"
)

type config struct {
	uri                   string
	emulatedSubnet        string
	mobilityManager       string
	mobilityManagerConfig *etcd.Node
	september             string
	septemberConfig       *etcd.Node
}

func getConfig() (conf config, err error) {
	endpoint := os.Getenv("SQUIRREL_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:2379"
	}
	client := etcd.NewClient([]string{endpoint})

	var ifce string
	ifce = "enp0s25"
	if err != nil {
		return
	}

	var addr net.IP
	addr, err = getAddr(ifce)
	if err != nil {
		return
	}
	conf.uri = addr.String() + ":1234"

	_, err = client.Set("/squirrel/master_ip", addr.String(), 0)
	if err != nil {
		return
	}
	_, err = client.Set("/squirrel/master_uri", conf.uri, 0)
	if err != nil {
		return
	}

	_, err = client.Set("/squirrel/master/emulated_subnet", "10.0.4.0/24", 0)
	conf.emulatedSubnet, err = common.GetEtcdValue(client, "/squirrel/master/emulated_subnet")
	if err != nil {
		return
	}

	_, err = client.Set("/squirrel/master/StaticUniformPositions.1/spacing", "200", 0)

	_, err = client.Set("/squirrel/master/StaticUniformPositions.1/shape", "Linear", 0)

	_, err = client.Set("/squirrel/master/mobility_manager", "StaticUniformPositions", 0)
	if err != nil {
		return
	}
	conf.emulatedSubnet, err = common.GetEtcdValue(client, "/squirrel/master/emulated_subnet")
	if err != nil {
		return
	}

	conf.mobilityManager, err = common.GetEtcdValue(client, "/squirrel/master/mobility_manager")
	if err != nil {
		return
	}

	var mobilityManagerConfigPath string
	mobilityManagerConfigPath, err = common.GetEtcdValue(client, "/squirrel/master/mobility_manager_config_path")
	if err != nil {
		if common.IsEtcdNotFoundError(err) {
			err = nil
		} else {
			return
		}
	} else {
		var resp *etcd.Response
		resp, err = client.Get(mobilityManagerConfigPath, false, true)
		if err != nil {
			return
		}
		if !resp.Node.Dir {
			err = errors.New("mobilityManagerConfig is not a Dir node")
			return
		}
		conf.mobilityManagerConfig = resp.Node
	}

	_, err = client.Set("/squirrel/master/september", "September0th", 0)
	conf.september, err = common.GetEtcdValue(client, "/squirrel/master/september")
	if err != nil {
		return
	}

	var septemberConfigPath string
	septemberConfigPath, err = common.GetEtcdValue(client, "/squirrel/master/september_config_path")
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

func runMaster(conf config, config *Config) (err error) {
	var network *net.IPNet
	_, network, err = net.ParseCIDR(conf.emulatedSubnet)
	if err != nil {
		return
	}

	var mobilityManager squirrel.MobilityManager
	mobilityManager, err = newMobilityManager(conf.mobilityManager)
	if err != nil {
		return
	}
	var september squirrel.September
	september, err = newSeptember(conf.september)
	if err != nil {
		return
	}

	err = september.Configure(conf.septemberConfig)
	if err != nil {
		log.Println("Creating September failed. Following message might help:\n")
		log.Println(september.ParametersHelp())
		return
	}

	master := NewMaster(network, mobilityManager, september, config)
	return master.Run(conf.uri)
}

func printHelp() {
	fmt.Println()
	fmt.Printf("Usage: %s\n", os.Args[0])
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("    SQUIRREL_ENDPOINT  : etcd endpoint UIR. [Optional]")
	fmt.Println("                             Default: http://127.0.0.1:4001")
	fmt.Println("Etcd Configuration Entries:")
	fmt.Println("    /squirrel/master/emulated_subnet              [Required]")
	fmt.Println("        Network in CIDR notation for emulated wireless network.")
	fmt.Println("    /squirrel/master/mobility_manager             [Required]")
	fmt.Println("        Name of the Mobility Manager.")
	fmt.Println("    /squirrel/master/mobility_manager_config_path [Optional]")
	fmt.Println("        Configuration node (a Dir) of the Mobility Manager.")
	fmt.Println("    /squirrel/master/september                    [Required]")
	fmt.Println("        Name of the September.")
	fmt.Println("    /squirrel/master/september_config_path        [Optional]")
	fmt.Println("        Configuration node (a Dir) of the September.")
}

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file; if specified, squirrel-master runs for 60 seconds and exits.")
var debug = flag.Bool("debug", false, "verbose logging for debug purposes")

func main() {
	log.SetOutput(os.Stdout)

	flag.Parse()
	flags := flag.NewFlagSet("server", flag.ContinueOnError)

	host := flags.String("host", "", "")
	port := flags.String("port", "", "")
	key := flags.String("key", "", "")
	authfile := flags.String("authfile", "", "")

	if *host == "" {
		*host = os.Getenv("HOST")
	}
	if *host == "" {
		*host = "172.31.0.209"
	}

	if *port == "" {
		*port = os.Getenv("PORT")
	}
	if *port == "" {
		*port = "1234"
	}

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		go func() {
			time.Sleep(60 * time.Second)
			pprof.StopCPUProfile()
			os.Exit(0)
		}()
	}

	conf, err := getConfig()
	if err != nil {
		log.Println(err)
		printHelp()
		os.Exit(1)
	}

	err = runMaster(conf, &Config{
		KeySeed:  *key,
		AuthFile: *authfile,
	})
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	return
}
