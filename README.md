# FakeConsole

> 轻量级虚拟主机实例生成工具，支持在路由器或 Linux 环境下模拟 PS4、SteamDeck 和 Nintendo Switch 等设备的网络环境。

## 🔰 一键命令
```bash
curl -sL https://gh-proxy.com/https://raw.githubusercontent.com/gdfsnhsw/FakeConsole/main/runfake.sh -o runfake.sh && chmod +x runfake.sh && ./runfake.sh -h
```

## 🚀 快速开始

### 下载后，赋予执行权限
```bash
chmod +x runfake.sh
```
### 基础运行格式
```bash
./runfake.sh -ip <IP_ADDRESS> [可选参数]
```

## ⚙️ 参数说明
| 参数 | 值类型 | 描述 | 默认值 / 备注 |
| :--- | :--- | :--- | :--- |
| `-ip` | `<IP>` | 必须设置。虚拟接口的 IP 地址。 | 示例 192.168.1.2 |
| `-type` | `<Type>` | 设备类型。可选值：`ps4`, `steamdeck`, `switch` (或简写 `ns`) | 默认为 `ps4` |
| `-mac` | `<MAC>` | 手动指定 MAC 地址。 | 默认根据 IP 地址自动生成 |
| `-gw` | `<GW>` | 手动指定网关 IP。 | 默认自动获取环境网关 |
| `-name` | `<Name>` | 手动指定当前实例的名称，方便管理。 | 默认自动生成 |
| `-autostart` | 无 | 将启动命令添加到 `/etc/rc.local`，实现开机自启动。 | 需系统支持 `rc.local` |
| `-clean-all` | 无 | 清理并删除所有已创建的实例和虚拟网卡 (可简写为 `-clean`)。 | **危险操作** |
| `-h`, `--help` | 无 | 在终端输出帮助信息。 | - |

## 💡 使用示例
### 1. 创建一个基础的 PS4 实例 (仅需 IP)
```bash
./runfake.sh -ip 192.168.1.50
```
### 2. 创建一个 Switch 实例，并设置开机自启
```bash
./runfake.sh -ip 192.168.1.51 -type switch -autostart
```
### 3. 自定义 MAC 和网关创建一个 SteamDeck 实例机自启
```bash
./runfake.sh -ip 192.168.1.52 -type steamdeck -mac 00:11:22:33:44:55 -gw 192.168.1.1
```
### 4. 一键清理所有创建的虚拟网卡和实例
```bash
./runfake.sh -clean-all
```


