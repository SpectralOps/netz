package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const (
	f_IPAddr int = iota
	f_HWType
	f_Flags
	f_HWAddr
	f_Mask
	f_Device
)

func getRouterARP() string {
	f, err := os.Open("/proc/net/arp")

	if err != nil {
		panic(err)
	}

	defer f.Close()

	s := bufio.NewScanner(f)
	s.Scan() // skip the field descriptions
	arp := ""
	for s.Scan() {
		line := s.Text()
		fields := strings.Fields(line)
		arp = fields[f_HWAddr]
	}

	return arp
}

const MASSCAN_CONF = `adapter[%s] = %s
router-mac[%s] = %s
adapter-ip[%s] = %s
adapter-mac[%s] = %s
`

func main() {
	conf := ""
	routerMAC := getRouterARP()
	interfaces, _ := net.Interfaces()
	currentIP := ""
	var macAddress net.HardwareAddr
	adapterName := ""
	index := 0

	for _, interf := range interfaces {
		found := false
		if addrs, err := interf.Addrs(); err == nil {
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						currentIP = ipnet.IP.String()
					}
				}

				netInterface, err := net.InterfaceByName(interf.Name)

				if err != nil {
					fmt.Println(err)
				}

				adapterName = netInterface.Name
				if currentIP != "" && (strings.HasPrefix(adapterName, "eth") || strings.HasPrefix(adapterName, "ens")) {
					found = true
				}
				macAddress = netInterface.HardwareAddr
			}
			if found {
				indexStr := strconv.Itoa(index)
				conf += fmt.Sprintf(MASSCAN_CONF, indexStr, adapterName, indexStr, routerMAC, indexStr, currentIP, indexStr, macAddress)
				index++
			}
		}
	}

	fmt.Println(conf)
}
