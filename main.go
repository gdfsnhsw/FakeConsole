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
// PS4 / SteamDeck / Switch (UDP 987) 文本协议
// ==========================================
const (
	ps4Tpl = "HTTP/1.1 200 OK\r\nhost-id:%s\r\nhost-type:PS4\r\nhost-name:FakePS4\r\nhost-request-port:%d\r\ndevice-discovery-protocol-version:00020020\r\nsystem-version:07020001\r\nrunning-app-name:Youtube\r\nrunning-app-titleid:CUSA01116\r\n"
	steamdeckTpl = "HTTP/1.1 200 OK\r\nhost-id:%s\r\nhost-type:SteamDeck\r\nhost-name:FakeSteamDeck\r\nhost-request-port:%d\r\ndevice-discovery-protocol-version:00030030\r\nsystem-version:01010001\r\nrunning-app-name:Steam\r\nrunning-app-titleid:STEAM001\r\n"
	switchTpl = "HTTP/1.1 200 OK\r\nhost-id:%s\r\nhost-type:NintendoSwitch\r\nhost-name:NintendoSwitch\r\nhost-request-port:%d\r\ndevice-discovery-protocol-version:00020020\r\nsystem-version:16.0.3\r\nrunning-app-name:MarioKart8\r\nrunning-app-titleid:0100152000022000\r\n"
)

// ==========================================
// Xbox SSDP (UDP 1900) 协议模板
// ==========================================
const xboxSSDPTpl = "HTTP/1.1 200 OK\r\n" +
	"CACHE-CONTROL: max-age=1800\r\n" +
	"EXT:\r\n" +
	"LOCATION: http://%s:2869/upnphost/udhisapi.dll?content=uuid:DE305D54-75B4-431B-ADB2-EB6B9E546014\r\n" +
	"SERVER: Microsoft-Windows/10.0 UPnP/1.0\r\n" +
	"ST: urn:schemas-upnp-org:device:MediaRenderer:1\r\n" +
	"USN: uuid:DE305D54-75B4-431B-ADB2-EB6B9E546014::urn:schemas-upnp-org:device:MediaRenderer:1\r\n\r\n"

func generateHostID() string {
	interfaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range interfaces {
			if iface.Flags&net.FlagLoopback == 0 && iface.Flags&net.FlagUp != 0 && len(iface.HardwareAddr) > 0 {
				return strings.ToUpper(hex.EncodeToString(iface.HardwareAddr))
			}
		}
	}
	b := make([]byte, 6)
	rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "192.168.1.100"
	}
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "192.168.1.100"
}

// 运行文本协议 (PS4/Switch等)
func runTextProtocolServer(deviceType string) {
	addr, _ := net.ResolveUDPAddr("udp", ":987")
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("UDP 987 监听失败: %v", err)
	}
	defer conn.Close()
	log.Printf("Listening on UDP :987 for %s", strings.ToUpper(deviceType))

	buf := make([]byte, 1024)
	for {
		_, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil { continue }
		hostID := generateHostID()
		var payload []byte
		switch deviceType {
		case "ps4": payload = []byte(fmt.Sprintf(ps4Tpl, hostID, remoteAddr.Port))
		case "steamdeck": payload = []byte(fmt.Sprintf(steamdeckTpl, hostID, remoteAddr.Port))
		case "switch", "ns": payload = []byte(fmt.Sprintf(switchTpl, hostID, remoteAddr.Port))
		}
		conn.WriteToUDP(payload, remoteAddr)
	}
}

// Xbox SmartGlass (UDP 5050)
func encodeSGString(s string) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint16(len(s)))
	buf.WriteString(s)
	buf.WriteByte(0x00)
	return buf.Bytes()
}

func runXboxSmartGlassServer() {
	addr, _ := net.ResolveUDPAddr("udp", ":5050")
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Printf("UDP 5050 监听失败 (SmartGlass): %v", err)
		return
	}
	defer conn.Close()
	log.Printf("Listening on UDP :5050 for Xbox SmartGlass")

	buf := make([]byte, 1024)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil { continue }
		if n >= 2 && binary.BigEndian.Uint16(buf[0:2]) == 0xDD00 {
			payload := new(bytes.Buffer)
			binary.Write(payload, binary.BigEndian, uint32(1))
			binary.Write(payload, binary.BigEndian, uint16(1))
			binary.Write(payload, binary.BigEndian, uint16(0))
			payload.Write(encodeSGString("FakeXbox"))
			payload.Write(encodeSGString("DE305D54-75B4-431B-ADB2-EB6B9E546014"))
			payload.Write(encodeSGString("dummy_cert")) // 这里依然是假证书

			payloadBytes := payload.Bytes()
			header := make([]byte, 6)
			binary.BigEndian.PutUint16(header[0:2], 0xDD01)
			binary.BigEndian.PutUint16(header[2:4], uint16(len(payloadBytes)))
			binary.BigEndian.PutUint16(header[4:6], 0x0000)

			conn.WriteToUDP(append(header, payloadBytes...), remoteAddr)
		}
	}
}

// Xbox SSDP (UDP 1900)
func runXboxSSDPServer() {
	// 加入 SSDP 组播地址
	addr, _ := net.ResolveUDPAddr("udp", "239.255.255.250:1900")
	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		log.Printf("UDP 1900 监听失败 (SSDP): %v. 可能是端口被占用或无权限。", err)
		return
	}
	defer conn.Close()
	log.Printf("Listening on UDP :1900 for Xbox SSDP Multicast")

	buf := make([]byte, 2048)
	localIP := getLocalIP()
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil { continue }
		
		reqStr := string(buf[:n])
		// 如果收到 M-SEARCH 并且目标是寻找所有设备或特定媒体渲染设备
		if strings.Contains(reqStr, "M-SEARCH") && (strings.Contains(reqStr, "ssdp:all") || strings.Contains(reqStr, "MediaRenderer")) {
			// 发送 Xbox 的 SSDP 响应
			resp := fmt.Sprintf(xboxSSDPTpl, localIP)
			conn.WriteToUDP([]byte(resp), remoteAddr)
		}
	}
}

func main() {
	deviceType := flag.String("type", "ps4", "Emulate device: ps4, steamdeck, switch, xbox")
	flag.Parse()
	dt := strings.ToLower(*deviceType)

	if dt == "xbox" || dt == "xsx" || dt == "xss" {
		// Xbox 需要同时跑 SmartGlass 和 SSDP 两个监听
		go runXboxSmartGlassServer()
		runXboxSSDPServer()
	} else if dt == "ps4" || dt == "steamdeck" || dt == "switch" || dt == "ns" {
		runTextProtocolServer(dt)
	} else {
		log.Fatalf("Unknown device: %s", dt)
	}
}
