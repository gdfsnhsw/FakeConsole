package main

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
)

// ==========================================
// PS4 / SteamDeck / Switch (UDP 987) 文本协议部分
// ==========================================

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
)

// generateHostID 根据 MAC 地址生成一个 host-id (用于 UDP 987 协议)
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
	b := make([]byte, 6)
	rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}

func runTextProtocolServer(deviceType string) {
	listenAddr := ":987"
	laddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		log.Fatalf("Failed to resolve UDP address: %v", err)
	}

	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		log.Fatalf("Failed to listen on UDP %s: %v", listenAddr, err)
	}
	defer conn.Close()

	log.Printf("Listening on UDP %s (Text Protocol) to emulate %s.", listenAddr, strings.ToUpper(deviceType))

	buf := make([]byte, 1500)
	for {
		_, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		hostID := generateHostID()
		var payload []byte

		switch deviceType {
		case "ps4":
			payload = []byte(fmt.Sprintf(ps4Tpl, hostID, remoteAddr.Port))
		case "steamdeck":
			payload = []byte(fmt.Sprintf(steamdeckTpl, hostID, remoteAddr.Port))
		case "switch", "ns":
			payload = []byte(fmt.Sprintf(switchTpl, hostID, remoteAddr.Port))
		}

		conn.WriteToUDP(payload, remoteAddr)
	}
}

// ==========================================
// Xbox (UDP 5050) SmartGlass 二进制协议部分
// ==========================================

// encodeSGString 实现 Xbox SmartGlass 的 SGString 编码
func encodeSGString(s string) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint16(len(s)))
	buf.WriteString(s)
	buf.WriteByte(0x00)
	return buf.Bytes()
}

func runXboxServer() {
	listenAddr := ":5050"
	laddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		log.Fatalf("Failed to resolve UDP address: %v", err)
	}

	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		log.Fatalf("Failed to listen on UDP %s: %v", listenAddr, err)
	}
	defer conn.Close()

	log.Printf("Listening on UDP %s (SmartGlass Protocol) to emulate XBOX.", listenAddr)

	buf := make([]byte, 1024)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		// 检查是否为 Xbox 发现请求 (0xDD00)
		if n >= 2 && binary.BigEndian.Uint16(buf[0:2]) == 0xDD00 {
			// log.Printf("收到来自 %s 的 Xbox 发现请求，发送伪造响应", remoteAddr)
			sendXboxDiscoveryResponse(conn, remoteAddr)
		}
	}
}

func sendXboxDiscoveryResponse(conn *net.UDPConn, remoteAddr *net.UDPAddr) {
	payload := new(bytes.Buffer)

	// 1. Primary Device Flags (uint32) - 0x00
	binary.Write(payload, binary.BigEndian, uint32(1))
	// 2. Device Type (uint16) - 0x04 (1 代表 Xbox One/Series)
	binary.Write(payload, binary.BigEndian, uint16(1))
	// 补齐字段对齐
	binary.Write(payload, binary.BigEndian, uint16(0))

	// 3. Console Name (SGString) - 0x08
	payload.Write(encodeSGString("FakeXbox"))
	// 4. UUID (SGString)
	payload.Write(encodeSGString("DE305D54-75B4-431B-ADB2-EB6B9E546014"))
	// 5. Certificate (SGString/Bytes) - 占位符
	payload.Write(encodeSGString("dummy_certificate_placeholder"))

	payloadBytes := payload.Bytes()

	// 构建 Packet Header (6 bytes)
	header := make([]byte, 6)
	binary.BigEndian.PutUint16(header[0:2], 0xDD01)                    // Packet Type: Discovery Response
	binary.BigEndian.PutUint16(header[2:4], uint16(len(payloadBytes))) // Unprotected Payload Length
	binary.BigEndian.PutUint16(header[4:6], 0x0000)                    // Version: Discovery 包固定为 0

	packet := append(header, payloadBytes...)
	conn.WriteToUDP(packet, remoteAddr)
}

// ==========================================
// 主程序入口
// ==========================================

func main() {
	deviceType := flag.String("type", "ps4", "The device type to emulate (ps4, steamdeck, switch, xbox).")
	flag.Parse()

	dt := strings.ToLower(*deviceType)

	// 根据传入的设备类型，启动对应的协议服务
	switch dt {
	case "xbox", "xsx", "xss":
		runXboxServer()
	case "ps4", "steamdeck", "switch", "ns":
		runTextProtocolServer(dt)
	default:
		log.Fatalf("Unknown device type: %s. Supported: ps4, steamdeck, switch, xbox", dt)
	}
}
