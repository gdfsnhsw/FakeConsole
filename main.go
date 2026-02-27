package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
)

// ================= 1. 响应模板区 =================

const (
	ps4Tpl = "HTTP/1.1 200 OK\r\nhost-id:%s\r\nhost-type:PS4\r\nhost-name:FakePS4\r\nhost-request-port:%d\r\ndevice-discovery-protocol-version:00020020\r\nsystem-version:07020001\r\nrunning-app-name:Youtube\r\nrunning-app-titleid:CUSA01116\r\n\r\n"

	steamdeckTpl = "HTTP/1.1 200 OK\r\nhost-id:%s\r\nhost-type:SteamDeck\r\nhost-name:FakeSteamDeck\r\nhost-request-port:%d\r\ndevice-discovery-protocol-version:00030030\r\nsystem-version:01010001\r\nrunning-app-name:Steam\r\nrunning-app-titleid:STEAM001\r\n\r\n"

	switchTpl = "HTTP/1.1 200 OK\r\nhost-id:%s\r\nhost-type:NintendoSwitch\r\nhost-name:NintendoSwitch\r\nhost-request-port:%d\r\ndevice-discovery-protocol-version:00020020\r\nsystem-version:16.0.3\r\nrunning-app-name:MarioKart8\r\nrunning-app-titleid:0100152000022000\r\n\r\n"

	// UU加速器专属 Xbox SSDP 模板
	uuXboxTpl = "HTTP/1.1 200 OK\r\n" +
		"CACHE-CONTROL: max-age=1800\r\n" +
		"ST: urn:schemas-upnp-org:device:Xbox-Remote-Protocol:1\r\n" +
		"USN: uuid:%s::urn:schemas-upnp-org:device:Xbox-Remote-Protocol:1\r\n" +
		"EXT:\r\n" +
		"SERVER: Microsoft-Windows-NT/10.0 UPnP/1.0\r\n" +
		"MAC:%s\r\n" + // UU 强依赖这个伪造的 MAC 头
		"host-id:%s\r\n" +
		"host-type:XboxSeriesX\r\n" +
		"host-name:Xbox-UU-Emu\r\n\r\n"
)

// ================= 2. 工具函数区 =================

func generateHostID() string {
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

// 强制生成 UU 信任的微软 MAC 地址 (50:1A:A5 开头)
func generateMicrosoftMAC() string {
	b := make([]byte, 3)
	rand.Read(b)
	return fmt.Sprintf("501AA5%02X%02X%02X", b[0], b[1], b[2])
}

func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return strings.ToUpper(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]))
}

// ================= 3. 主函数与路由 =================

func main() {
	devicesFlag := flag.String("type", "xbox", "要伪装的设备: ps4, steamdeck, switch, xbox, 或 all")
	flag.Parse()

	var textDevices []string
	var enableXbox bool

	// 解析逗号分隔的参数
	for _, d := range strings.Split(*devicesFlag, ",") {
		d = strings.ToLower(strings.TrimSpace(d))
		switch d {
		case "ps4", "steamdeck", "switch", "ns":
			if d == "ns" { d = "switch" }
			textDevices = append(textDevices, d)
		case "xbox", "xbx":
			enableXbox = true
		case "all":
			textDevices = []string{"ps4", "steamdeck", "switch"}
			enableXbox = true
		}
	}

	var wg sync.WaitGroup

	// 启动 UDP 987 监听 (服务于 PS4 / Switch / SteamDeck)
	if len(textDevices) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			startTextServer(textDevices)
		}()
	}

	// 启动 UDP 1900 监听 (专供 UU 加速器扫 Xbox)
	if enableXbox {
		wg.Add(1)
		go func() {
			defer wg.Done()
			startXboxUUServer()
		}()
	}

	if len(textDevices) == 0 && !enableXbox {
		log.Fatal("❌ 未指定任何有效的伪装设备，程序退出。")
	}

	log.Println("✅ 伪装服务已启动！等待 UU 加速器扫描...")
	wg.Wait()
}

// ================= 4. 网络服务实现 =================

func startTextServer(devices []string) {
	addr, _ := net.ResolveUDPAddr("udp", ":987")
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Printf("❌ 无法监听 UDP 987: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("🟢 [文本引擎] 监听 UDP 987 (设备: %s)", strings.Join(devices, ", "))
	buf := make([]byte, 1500)

	for {
		_, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil { continue }

		hostID := generateHostID()
		for _, dev := range devices {
			var payload []byte
			switch dev {
			case "ps4":
				payload = []byte(fmt.Sprintf(ps4Tpl, hostID, remoteAddr.Port))
			case "steamdeck":
				payload = []byte(fmt.Sprintf(steamdeckTpl, hostID, remoteAddr.Port))
			case "switch":
				payload = []byte(fmt.Sprintf(switchTpl, hostID, remoteAddr.Port))
			}
			conn.WriteToUDP(payload, remoteAddr)
		}
	}
}

func startXboxUUServer() {
	addr, _ := net.ResolveUDPAddr("udp", ":1900")
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Printf("❌ 无法监听 UDP 1900。若在 Windows 上运行，请在服务(services.msc)中禁用 'SSDP Discovery' 服务: %v", err)
		return
	}
	defer conn.Close()

	log.Println("🟢 [Xbox引擎] 监听 UDP 1900 (专供 UU加速器 SSDP 识别)")
	fakeMAC := generateMicrosoftMAC()
	fakeUUID := generateUUID()
	buf := make([]byte, 2048)

	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil { continue }

		reqStr := string(buf[:n])
		// 拦截 UU加速器 发出的 M-SEARCH 广播包
		if strings.HasPrefix(reqStr, "M-SEARCH") {
			payload := []byte(fmt.Sprintf(uuXboxTpl, fakeUUID, fakeMAC, fakeMAC))
			conn.WriteToUDP(payload, remoteAddr)
		}
	}
}
