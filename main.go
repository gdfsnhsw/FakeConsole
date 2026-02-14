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
// 注意：原代码中缺少反引号，这里已修复，使用反引号 ` 定义多行字符串
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

	// 新增：Nintendo Switch 模板
	// running-app-titleid 使用了马里奥赛车8的ID (0100152000022000) 作为示例，这通常能被加速器识别
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

func main() {
	// 更新命令行参数说明，增加 switch 选项
	deviceType := flag.String("type", "ps4", "The device type to emulate (ps4, steamdeck, switch).")
	flag.Parse()

	// 监听端口保持不变
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

	log.Printf("Listening on %s (all interfaces) to emulate %s.", listenAddr, strings.ToUpper(*deviceType))

	buf := make([]byte, 1500)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Error reading from UDP: %v", err)
			continue
		}

		// 简单的日志，避免刷屏
		// log.Printf("Received discovery packet from %s", remoteAddr)
		sendResponse(conn, remoteAddr, *deviceType)
	}
}

func sendResponse(conn *net.UDPConn, remoteAddr *net.UDPAddr, deviceType string) {
	// 每次响应重新获取或生成 ID (如果是基于 MAC 的话其实是一样的)
	hostID := generateHostID()
	var payload []byte
	var err error

	// 转换为小写以防用户输入 "Switch" 或 "SWITCH"
	switch strings.ToLower(deviceType) {
	case "ps4":
		payload = []byte(fmt.Sprintf(ps4Tpl, hostID, remoteAddr.Port))
	case "steamdeck":
		payload = []byte(fmt.Sprintf(steamdeckTpl, hostID, remoteAddr.Port))
	case "switch", "ns": // 支持 switch 或 ns 简写
		payload = []byte(fmt.Sprintf(switchTpl, hostID, remoteAddr.Port))
	default:
		log.Printf("Unknown device type: %s. Supported: ps4, steamdeck, switch", deviceType)
		return
	}

	_, err = conn.WriteToUDP(payload, remoteAddr)
	if err != nil {
		log.Printf("Failed to send response to %s: %v", remoteAddr, err)
	} else {
		// log.Printf("Sent %s response to %s", strings.ToUpper(deviceType), remoteAddr)
	}
}
