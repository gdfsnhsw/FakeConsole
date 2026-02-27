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

// 全局使用的真实化 Xbox 识别码
const (
	xboxUUID       = "F4A3928B-1C7D-4E5F-A8B9-6C2D7E4F1A3B" // 真实的 UUID 格式
	xboxSysVersion = "10.0.25398.2258"                     // 真实的 Xbox OS 版本号
	xboxTitleID    = "71402288"                            // Xbox Home 桌面 Title ID
)

// ==========================================
// UDP 987 文本协议模板
// ==========================================
const (
	ps4Tpl = "HTTP/1.1 200 OK\r\nhost-id:%s\r\nhost-type:PS4\r\nhost-name:FakePS4\r\nhost-request-port:%d\r\ndevice-discovery-protocol-version:00020020\r\nsystem-version:07020001\r\nrunning-app-name:Youtube\r\nrunning-app-titleid:CUSA01116\r\n"
	steamdeckTpl = "HTTP/1.1 200 OK\r\nhost-id:%s\r\nhost-type:SteamDeck\r\nhost-name:FakeSteamDeck\r\nhost-request-port:%d\r\ndevice-discovery-protocol-version:00030030\r\nsystem-version:01010001\r\nrunning-app-name:Steam\r\nrunning-app-titleid:STEAM001\r\n"
	switchTpl = "HTTP/1.1 200 OK\r\nhost-id:%s\r\nhost-type:NintendoSwitch\r\nhost-name:NintendoSwitch\r\nhost-request-port:%d\r\ndevice-discovery-protocol-version:00020020\r\nsystem-version:16.0.3\r\nrunning-app-name:MarioKart8\r\nrunning-app-titleid:0100152000022000\r\n"
	// 针对可能扫描 987 端口的魔改版 Xbox 识别
	xboxTextTpl = "HTTP/1.1 200 OK\r\nhost-id:%s\r\nhost-type:Xbox\r\nhost-name:XboxSeriesX\r\nhost-request-port:%d\r\ndevice-discovery-protocol-version:00020020\r\nsystem-version:%s\r\nrunning-app-name:XboxHome\r\nrunning-app-titleid:%s\r\n"
)

// ==========================================
// Xbox SSDP (UDP 1900) 协议模板
// ==========================================
const xboxSSDPTpl = "HTTP/1.1 200 OK\r\n" +
	"CACHE-CONTROL: max-age=1800\r\n" +
	"EXT:\r\n" +
	"LOCATION: http://%s:2869/upnphost/udhisapi.dll?content=uuid:%s\r\n" +
	"SERVER: Microsoft-Windows/10.0 UPnP/1.0\r\n" +
	"ST: urn:schemas-upnp-org:device:MediaRenderer:1\r\n" +
	"USN: uuid:%s::urn:schemas-upnp-org:device:MediaRenderer:1\r\n\r\n"

// 工具函数：生成伪造 MAC 转换的 HostID
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

// 工具函数：获取本机局域网 IP
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

// ==========================================
// 1. 运行文本协议服务 (UDP 987)
// ==========================================
func runTextProtocolServer(deviceType string) {
	addr, _ := net.ResolveUDPAddr("udp", ":987")
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("UDP 987 监听失败: %v", err)
	}
	defer conn.Close()
	log.Printf("[UDP 987] Listening for %s (Text Protocol)", strings.ToUpper(deviceType))

	buf := make([]byte, 1024)
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
		case "xbox", "xsx", "xss":
			payload = []byte(fmt.Sprintf(xboxTextTpl, hostID, remoteAddr.Port, xboxSysVersion, xboxTitleID))
		}
		conn.WriteToUDP(payload, remoteAddr)
	}
}

// ==========================================
// 2. 运行 Xbox SmartGlass 服务 (UDP 5050)
// ==========================================
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
		log.Printf("[UDP 5050] 监听失败 (SmartGlass): %v", err)
		return
	}
	defer conn.Close()
	log.Printf("[UDP 5050] Listening for Xbox SmartGlass Discovery")

	buf := make([]byte, 1024)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		if n >= 2 && binary.BigEndian.Uint16(buf[0:2]) == 0xDD00 {
			payload := new(bytes.Buffer)
			binary.Write(payload, binary.BigEndian, uint32(1)) // Primary Device Flags
			binary.Write(payload, binary.BigEndian, uint16(1)) // Device Type (1=Xbox)
			binary.Write(payload, binary.BigEndian, uint16(0)) // Padding
			payload.Write(encodeSGString("XboxSeriesX"))       // Console Name
			payload.Write(encodeSGString(xboxUUID))            // 注入真实格式的 UUID
			payload.Write(encodeSGString("dummy_cert_auth_bypass")) // 占位证书

			payloadBytes := payload.Bytes()
			header := make([]byte, 6)
			binary.BigEndian.PutUint16(header[0:2], 0xDD01)
			binary.BigEndian.PutUint16(header[2:4], uint16(len(payloadBytes)))
			binary.BigEndian.PutUint16(header[4:6], 0x0000)

			conn.WriteToUDP(append(header, payloadBytes...), remoteAddr)
		}
	}
}

// ==========================================
// 3. 运行 Xbox SSDP 服务 (UDP 1900)
// ==========================================
func runXboxSSDPServer() {
	addr, _ := net.ResolveUDPAddr("udp", "239.255.255.250:1900")
	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		log.Printf("[UDP 1900] 监听组播失败 (SSDP): %v (若是 Docker，请确保使用 --network host)", err)
		return
	}
	defer conn.Close()
	log.Printf("[UDP 1900] Listening for Xbox SSDP Multicast")

	buf := make([]byte, 2048)
	localIP := getLocalIP()
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		reqStr := string(buf[:n])
		if strings.Contains(reqStr, "M-SEARCH") && (strings.Contains(reqStr, "ssdp:all") || strings.Contains(reqStr, "MediaRenderer")) {
			// 注入本机 IP 和 真实的 UUID
			resp := fmt.Sprintf(xboxSSDPTpl, localIP, xboxUUID, xboxUUID)
			conn.WriteToUDP([]byte(resp), remoteAddr)
		}
	}
}

// ==========================================
// 主入口
// ==========================================
func main() {
	deviceType := flag.String("type", "ps4", "Emulate device: ps4, steamdeck, switch, xbox")
	flag.Parse()
	dt := strings.ToLower(*deviceType)

	if dt == "xbox" || dt == "xsx" || dt == "xss" {
		// Xbox 模式：火力全开，同时启动三种特征伪装
		go runXboxSmartGlassServer()
		go runXboxSSDPServer()
		go runTextProtocolServer(dt) // 有些加速器魔改了文本协议扫 Xbox
		
		// 阻塞主线程
		select {} 
	} else if dt == "ps4" || dt == "steamdeck" || dt == "switch" || dt == "ns" {
		// 其他设备只跑 987 端口
		runTextProtocolServer(dt)
	} else {
		log.Fatalf("Unknown device: %s", dt)
	}
}
