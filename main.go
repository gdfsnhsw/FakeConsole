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

// 设备响应模板
const (
	ps4Tpl = "HTTP/1.1 200 OK\r\nhost-id:%s\r\nhost-type:PS4\r\nhost-name:FakePS4\r\nhost-request-port:%d\r\ndevice-discovery-protocol-version:00020020\r\nsystem-version:07020001\r\nrunning-app-name:Youtube\r\nrunning-app-titleid:CUSA01116\r\n\r\n"

	steamdeckTpl = "HTTP/1.1 200 OK\r\nhost-id:%s\r\nhost-type:SteamDeck\r\nhost-name:FakeSteamDeck\r\nhost-request-port:%d\r\ndevice-discovery-protocol-version:00030030\r\nsystem-version:01010001\r\nrunning-app-name:Steam\r\nrunning-app-titleid:STEAM001\r\n\r\n"

	switchTpl = "HTTP/1.1 200 OK\r\nhost-id:%s\r\nhost-type:NintendoSwitch\r\nhost-name:NintendoSwitch\r\nhost-request-port:%d\r\ndevice-discovery-protocol-version:00020020\r\nsystem-version:16.0.3\r\nrunning-app-name:MarioKart8\r\nrunning-app-titleid:0100152000022000\r\n\r\n"

	// 增强型 Xbox 模板：加入 SSDP 标准字段和 UUID
	// %s1: UUID, %s2: UUID, %d: Port
	xboxTpl = "HTTP/1.1 200 OK\r\n" +
		"CACHE-CONTROL: max-age=1800\r\n" +
		"ST: urn:schemas-upnp-org:device:Xbox-Remote-Protocol:1\r\n" +
		"USN: uuid:%s::urn:schemas-upnp-org:device:Xbox-Remote-Protocol:1\r\n" +
		"SERVER: Microsoft-Windows-NT/10.0 UPnP/1.0\r\n" +
		"host-id:%s\r\n" +
		"host-type:XboxSeriesX\r\n" +
		"host-name:Xbox-Custom-Emu\r\n" +
		"host-request-port:%d\r\n" +
		"device-discovery-protocol-version:00040010\r\n" +
		"system-version:10.0.22621.3446\r\n" +
		"running-app-name:HaloInfinite\r\n" +
		"running-app-titleid:Microsoft.Tokyo_8wekyb3d8bbwe\r\n\r\n"
)

// generateMACID 生成传统的 12 位大写 MAC 地址字符串
func generateMACID() string {
	interfaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range interfaces {
			if iface.Flags&net.FlagLoopback == 0 && iface.Flags&net.FlagUp != 0 && len(iface.HardwareAddr) > 0 {
				return strings.ToUpper(hex.EncodeToString(iface.HardwareAddr))
			}
		}
	}
	bytes := make([]byte, 6)
	rand.Read(bytes)
	return strings.ToUpper(hex.EncodeToString(bytes))
}

// generateUUID 生成 Xbox 喜欢的 UUID 格式 (e.g., 550e8400-e29b-41d4-a716-446655440000)
func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func main() {
	deviceType := flag.String("type", "ps4", "Device to emulate (ps4, steamdeck, switch, xbox).")
	port := flag.Int("port", 987, "UDP port to listen on (try 1900 for SSDP if 987 fails).")
	flag.Parse()

	listenAddr := fmt.Sprintf(":%d", *port)
	laddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		log.Fatalf("Address error: %v", err)
	}

	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		log.Fatalf("Listen error: %v", err)
	}
	defer conn.Close()

	log.Printf("Listening on %s. Emulating %s.", listenAddr, strings.ToUpper(*deviceType))
	log.Println("Tip: If Xbox app still can't find it, try running with: -port 1900")

	buf := make([]byte, 1500)
	for {
		_, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		sendResponse(conn, remoteAddr, *deviceType)
	}
}

func sendResponse(conn *net.UDPConn, remoteAddr *net.UDPAddr, deviceType string) {
	var payload []byte
	macID := generateMACID()

	switch strings.ToLower(deviceType) {
	case "ps4":
		payload = []byte(fmt.Sprintf(ps4Tpl, macID, remoteAddr.Port))
	case "steamdeck":
		payload = []byte(fmt.Sprintf(steamdeckTpl, macID, remoteAddr.Port))
	case "switch", "ns":
		payload = []byte(fmt.Sprintf(switchTpl, macID, remoteAddr.Port))
	case "xbox", "xbx":
		uuid := generateUUID()
		// Xbox 需要 UUID 作为 host-id，且包含特定的 SSDP 头部
		payload = []byte(fmt.Sprintf(xboxTpl, uuid, uuid, remoteAddr.Port))
	default:
		log.Printf("Unknown device: %s", deviceType)
		return
	}

	_, err := conn.WriteToUDP(payload, remoteAddr)
	if err == nil {
		log.Printf("[%s] Responded to %s", strings.ToUpper(deviceType), remoteAddr)
	}
}
