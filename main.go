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

// 不同设备的响应模板 (文本协议)
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

// ================= 工具函数 =================

// generateHostID 生成 MAC 地址风格的 ID (用于文本协议)
func generateHostID() string {
	interfaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range interfaces {
			if iface.Flags&net.FlagLoopback == 0 && iface.Flags&net.FlagUp != 0 && len(iface.HardwareAddr) > 0 {
				return strings.ToUpper(hex.EncodeToString(iface.HardwareAddr))
			}
		}
	}
	log.Println("Warning: Could not find a suitable MAC address. Generating a random host-id as a fallback.")
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		log.Fatalf("Failed to generate random bytes for host ID: %v", err)
	}
	return strings.ToUpper(hex.EncodeToString(bytes))
}

// generateUUID 生成 Xbox 需要的 UUID 格式
func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return strings.ToUpper(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]))
}

// writePrefixedString Xbox 二进制协议专用的字符串写入函数 (带16位长度前缀和NULL结尾)
func writePrefixedString(buf *bytes.Buffer, str string) {
	strBytes := []byte(str)
	binary.Write(buf, binary.BigEndian, uint16(len(strBytes)))
	buf.Write(strBytes)
	buf.WriteByte(0x00)
}

// ================= 主函数 =================

func main() {
	deviceType := flag.String("type", "ps4", "The device type to emulate (ps4, steamdeck, switch, xbox).")
	portOverride := flag.Int("port", 0, "Override default port (0 means auto: xbox=5050, others=987)")
	flag.Parse()

	targetDevice := strings.ToLower(*deviceType)

	// 自动决定监听端口：Xbox 必须是 5050，其他默认 987
	port := *portOverride
	if port == 0 {
		if targetDevice == "xbox" || targetDevice == "xbx" {
			port = 5050
		} else {
			port = 987
		}
	}

	listenAddr := fmt.Sprintf(":%d", port)
	laddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		log.Fatalf("Failed to resolve UDP address: %v", err)
	}

	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		log.Fatalf("Failed to listen on UDP address: %v", err)
	}
	defer conn.Close()

	log.Printf("Listening on %s (all interfaces) to emulate %s.", listenAddr, strings.ToUpper(targetDevice))

	buf := make([]byte, 1500)
	for {
		// 注意这里接收了 n，因为 Xbox 二进制协议需要判断包长度
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Error reading from UDP: %v", err)
			continue
		}

		// 根据设备类型路由到不同的处理引擎
		if targetDevice == "xbox" || targetDevice == "xbx" {
			// Xbox SmartGlass 协议：必须校验头部是否为 0xDD00
			if n >= 2 && buf[0] == 0xDD && buf[1] == 0x00 {
				sendXboxBinaryResponse(conn, remoteAddr)
			}
		} else {
			// 文本协议直接响应
			sendTextResponse(conn, remoteAddr, targetDevice)
		}
	}
}

// ================= 响应处理引擎 =================

// sendTextResponse 处理 PS4, SteamDeck, Switch 的文本响应
func sendTextResponse(conn *net.UDPConn, remoteAddr *net.UDPAddr, deviceType string) {
	hostID := generateHostID()
	var payload []byte

	switch deviceType {
	case "ps4":
		payload = []byte(fmt.Sprintf(ps4Tpl, hostID, remoteAddr.Port))
	case "steamdeck":
		payload = []byte(fmt.Sprintf(steamdeckTpl, hostID, remoteAddr.Port))
	case "switch", "ns":
		payload = []byte(fmt.Sprintf(switchTpl, hostID, remoteAddr.Port))
	default:
		log.Printf("Unknown text device type: %s", deviceType)
		return
	}

	_, err := conn.WriteToUDP(payload, remoteAddr)
	if err != nil {
		log.Printf("Failed to send text response to %s: %v", remoteAddr, err)
	}
}

// sendXboxBinaryResponse 处理 Xbox One / Series X|S 的二进制响应
func sendXboxBinaryResponse(conn *net.UDPConn, remoteAddr *net.UDPAddr) {
	buf := new(bytes.Buffer)

	// 1. Packet Type (0xDD01: Discovery Response) & Flags
	binary.Write(buf, binary.BigEndian, uint16(0xDD01))
	binary.Write(buf, binary.BigEndian, uint16(0x0000))
	
	// 2. Client Type, Min/Max Version
	binary.Write(buf, binary.BigEndian, uint16(0x0001)) // Client Type (1 = Xbox One)
	binary.Write(buf, binary.BigEndian, uint16(0x0002)) // Min Version
	binary.Write(buf, binary.BigEndian, uint16(0x0002)) // Max Version

	// 3. 动态字符串: 主机名和 UUID
	writePrefixedString(buf, "FakeXbox-Emu")
	writePrefixedString(buf, generateUUID())

	// 4. Status / Last Error (正常状态全0)
	binary.Write(buf, binary.BigEndian, uint32(0x00000000))

	// 5. 伪造证书 (Dummy Certificate)
	dummyCert := make([]byte, 256)
	for i := range dummyCert {
		dummyCert[i] = byte(i % 255)
	}
	binary.Write(buf, binary.BigEndian, uint16(len(dummyCert))) // 证书长度
	buf.Write(dummyCert)                                        // 证书内容

	_, err := conn.WriteToUDP(buf.Bytes(), remoteAddr)
	if err != nil {
		log.Printf("Failed to send Xbox binary response to %s: %v", remoteAddr, err)
	} else {
		// 调试日志，证明触发了二进制协议
		// log.Printf("Sent XBOX binary response to %s", remoteAddr)
	}
}
