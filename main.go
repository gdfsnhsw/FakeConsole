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
running-app-titleid:CUSA01116`

	steamdeckTpl = `HTTP/1.1 200 OK
host-id:%s
host-type:SteamDeck
host-name:FakeSteamDeck
host-request-port:%d
device-discovery-protocol-version:00030030
system-version:01010001
running-app-name:Steam
running-app-titleid:STEAM001`

	switchTpl = `HTTP/1.1 200 OK
host-id:%s
host-type:NintendoSwitch
host-name:NintendoSwitch
host-request-port:%d
device-discovery-protocol-version:00020020
system-version:16.0.3
running-app-name:MarioKart8
running-app-titleid:0100152000022000`
)

// generateHostID 根据第一个活动的、非环回网络接口的 MAC 地址生成一个 host-id。
func generateHostID() string {
	interfaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range interfaces {
			// 排除环回接口、未启用的接口，且必须有 MAC 地址
			if iface.Flags&net.FlagLoopback == 0 && iface.Flags&net.FlagUp != 0 && len(iface.HardwareAddr) > 0 {
				return strings.ToUpper(hex.EncodeToString(iface.HardwareAddr))
			}
		}
	}

	log.Println("Warning: Could not find a suitable MAC address. Generating a random host-id.")
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		log.Fatalf("Failed to generate random bytes: %v", err)
	}
	return strings.ToUpper(hex.EncodeToString(bytes))
}

func main() {
	// 定义参数
	deviceType := flag.String("type", "ps4", "The device type to emulate (ps4, switch, steamdeck)")
	flag.Parse()

	dtype := strings.ToLower(*deviceType)
	var listenPort string

	// 根据设备类型自动分配端口
	switch dtype {
	case "ps4":
		listenPort = "987"
	case "switch", "ns":
		listenPort = "10008"
		dtype = "switch" // 统一名称用于后续逻辑
	case "steamdeck":
		listenPort = "10007" // 某些插件识别 SteamDeck 的常用端口
	default:
		log.Fatalf("Unknown device type: %s. Supported types: ps4, switch, steamdeck", *deviceType)
	}

	listenAddr := "0.0.0.0:" + listenPort
	laddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		log.Fatalf("Failed to resolve UDP address: %v", err)
	}

	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v. (Maybe try sudo?)", listenAddr, err)
	}
	defer conn.Close()

	log.Printf("Starting emulation: [%s] on UDP port [%s]", strings.ToUpper(dtype), listenPort)
	log.Println("Waiting for discovery packets from accelerator...")

	buf := make([]byte, 1500)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Error reading from UDP: %v", err)
			continue
		}

		log.Printf("Received %d bytes from %s. Sending %s response...", n, remoteAddr, strings.ToUpper(dtype))
		sendResponse(conn, remoteAddr, dtype)
	}
}

func sendResponse(conn *net.UDPConn, remoteAddr *net.UDPAddr, dtype string) {
	hostID := generateHostID()
	var payload string

	switch dtype {
	case "ps4":
		payload = fmt.Sprintf(ps4Tpl, hostID, remoteAddr.Port)
	case "steamdeck":
		payload = fmt.Sprintf(steamdeckTpl, hostID, remoteAddr.Port)
	case "switch":
		payload = fmt.Sprintf(switchTpl, hostID, remoteAddr.Port)
	}

	_, err := conn.WriteToUDP([]byte(payload), remoteAddr)
	if err != nil {
		log.Printf("Failed to send response: %v", err)
	}
}
