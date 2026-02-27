package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
)

// 不同设备的响应模板
const (
	ps4Tpl = `HTTP/1.1 200 OK
host-id:%s
host-type:PS4
host-name:FakePS4
host-request-port:%d
device-discovery-protocol-version:00020020
system-version:07020001
running-app-name:Youtube
running-app-titleid:CUSA01116
`
	steamdeckTpl = `HTTP/1.1 200 OK
host-id:%s
host-type:SteamDeck
host-name:FakeSteamDeck
host-request-port:%d
device-discovery-protocol-version:00030030
system-version:01010001
running-app-name:Steam
running-app-titleid:STEAM001
`
	switchTpl = `HTTP/1.1 200 OK
host-id:%s
host-type:NintendoSwitch
host-name:NintendoSwitch
host-request-port:%d
device-discovery-protocol-version:00020020
system-version:16.0.3
running-app-name:MarioKart8
running-app-titleid:0100152000022000
`
	// 新增：Xbox Series X/S 模板
	xboxTpl = `HTTP/1.1 200 OK
host-id:%s
host-type:XboxSeriesX
host-name:Xbox-Custom-Emu
host-request-port:%d
device-discovery-protocol-version:00040010
system-version:10.0.22621.3446
running-app-name:HaloInfinite
running-app-titleid:Microsoft.Tokyo_8wekyb3d8bbwe
`
)

func generateHostID() string {
	interfaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range interfaces {
			if iface.Flags&net.FlagLoopback == 0 && iface.Flags&net.FlagUp != 0 && len(iface.HardwareAddr) > 0 {
				return strings.ToUpper(hex.EncodeToString(iface.HardwareAddr))
			}
		}
	}

	log.Println("Warning: Could not find a suitable MAC address. Generating a random host-id.")
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		log.Fatalf("Failed to generate random bytes for host ID: %v", err)
	}
	return strings.ToUpper(hex.EncodeToString(bytes))
}

func main() {
	// 更新参数说明，加入 xbox
	deviceType := flag.String("type", "ps4", "The device type to emulate (ps4, steamdeck, switch, xbox).")
	flag.Parse()

	listenAddr := ":987"
	laddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		log.Fatalf("Failed to resolve UDP address: %v", err)
	}

	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		log.Fatalf("Failed to listen on UDP address: %v", err)
	}
	defer conn.Close()

	log.Printf("Listening on %s to emulate %s.", listenAddr, strings.ToUpper(*deviceType))

	buf := make([]byte, 1500)
	for {
		_, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Error reading from UDP: %v", err)
			continue
		}
		sendResponse(conn, remoteAddr, *deviceType)
	}
}

func sendResponse(conn *net.UDPConn, remoteAddr *net.UDPAddr, deviceType string) {
	hostID := generateHostID()
	var payload []byte

	switch strings.ToLower(deviceType) {
	case "ps4":
		payload = []byte(fmt.Sprintf(ps4Tpl, hostID, remoteAddr.Port))
	case "steamdeck":
		payload = []byte(fmt.Sprintf(steamdeckTpl, hostID, remoteAddr.Port))
	case "switch", "ns":
		payload = []byte(fmt.Sprintf(switchTpl, hostID, remoteAddr.Port))
	case "xbox", "xbx": // 新增 Xbox 分支
		payload = []byte(fmt.Sprintf(xboxTpl, hostID, remoteAddr.Port))
	default:
		log.Printf("Unknown device type: %s. Supported: ps4, steamdeck, switch, xbox", deviceType)
		return
	}

	_, err := conn.WriteToUDP(payload, remoteAddr)
	if err != nil {
		log.Printf("Failed to send response to %s: %v", remoteAddr, err)
	}
}
