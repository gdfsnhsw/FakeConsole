#!/bin/sh

# ==============================================================================
# FakeConsole 伪装游戏主机网络隔离脚本
# ==============================================================================

# === 颜色定义 ===
RED='\033[31m'
GREEN='\033[32m'
YELLOW='\033[33m'
CYAN='\033[36m'
PLAIN='\033[0m'
BOLD='\033[1m'

# === 全局配置 ===
BINARY_PATH="/root/fakeconsole"
BRIDGE_IF="br-lan"
AUTO_START=0

# === 日志封装函数 ===
log_info() {
    echo -e "${CYAN}[INFO]${PLAIN} $1"
}
log_success() {
    echo -e "${GREEN}[ OK ]${PLAIN} $1"
}
log_warn() {
    echo -e "${YELLOW}[WARN]${PLAIN} $1"
}
log_error() {
    echo -e "${RED}[FAIL]${PLAIN} $1"
}
print_kv() {
    echo -e "  * $1: ${CYAN}${BOLD}$2${PLAIN}"
}
print_hint() {
    echo -e "${YELLOW}>> 提示: 请运行 $0 -h 查看完整帮助与示例${PLAIN}"
}

# === 帮助信息 ===
show_help() {
    # 获取当前执行的脚本名称，确保示例命令名称与用户实际文件名一致
    local script_name="./$(basename "$0")"
    
    # 动态获取当前网桥的 IP 段信息
    local current_cidr=$(ip -o -4 addr show "$BRIDGE_IF" 2>/dev/null | awk 'NR==1 {print $4}')
    
    # 提取纯 IP 地址 (去掉 /24 等子网掩码后缀)
    local ip_addr="${current_cidr%/*}"
    # 提取前三个网段 (去掉最后一个 . 和后面的数字，例如 172.18.3.1 变成 172.18.3)
    local network_prefix="${ip_addr%.*}" 

    echo -e "${YELLOW}用法:${PLAIN}"
    echo "  ${script_name} -ip <IP地址> [选项...]"
    echo ""
    echo -e "${YELLOW}必选参数:${PLAIN}"
    if [ -n "$current_cidr" ]; then
        echo -e "  -ip <IP>        容器内部 IP ${GREEN}(推荐使用当前网段: ${network_prefix}.x)${PLAIN}"
    else
        echo "  -ip <IP>        容器内部 IP (例: 172.18.3.225)"
    fi
    echo ""
    echo -e "${YELLOW}可选参数:${PLAIN}"
    echo "  -type <Type>    设备类型: ps4(默认)、steamdeck、switch(或简写 ns)"
    echo "  -mac <MAC>      手动 MAC (默认根据 IP 自动生成)"
    echo "  -gw <GW>        手动网关 (默认自动获取)"
    echo "  -name <Name>    手动实例名 (默认自动生成)"
    echo "  -autostart      添加到 /etc/rc.local 实现开机自启"
    echo "  -clean-all      清理并删除所有已创建的实例和虚拟网卡 (简写 -clean)"
    echo "  -h, --help      显示帮助"
    
    # 追加网络环境提示块
    if [ -n "$current_cidr" ]; then
        echo ""
        echo -e "${YELLOW}>> 当前网络环境提示 <<${PLAIN}"
        echo -e "  检测到网桥 (${CYAN}${BRIDGE_IF}${PLAIN}) 的 IP 为: ${GREEN}${ip_addr}${PLAIN}"
        echo -e "  请确保您分配的 ${CYAN}-ip${PLAIN} 属于 ${GREEN}${network_prefix}.x${PLAIN} 网段，且未被局域网内其他设备占用！"
    fi

    # 追加具体命令示例
    echo ""
    echo -e "${YELLOW}>> 常用命令示例 <<${PLAIN}"
    if [ -n "$current_cidr" ]; then
        echo -e "  1. 基础启动并设置开机自启 (请将 x 替换为具体可用数字):"
        echo -e "     ${CYAN}${script_name} -autostart -ip ${network_prefix}.x${PLAIN}"
        echo -e "     ${PLAIN}(例如: ${GREEN}${script_name} -autostart -ip ${network_prefix}.251${PLAIN})"
        echo ""
        echo -e "  2. 模拟 Switch 并设置开机自启:"
        echo -e "     ${CYAN}${script_name} -type ns -autostart -ip ${network_prefix}.252${PLAIN}"
    else
        echo -e "  1. 基础启动并设置开机自启 (请将 x 替换为具体可用数字):"
        echo -e "     ${CYAN}${script_name} -autostart -ip 172.18.3.x${PLAIN}"
        echo -e "     ${PLAIN}(例如: ${GREEN}${script_name} -autostart -ip 172.18.3.251${PLAIN})"
        echo ""
        echo -e "  2. 模拟 Switch 并设置开机自启:"
        echo -e "     ${CYAN}${script_name} -type ns -autostart -ip 172.18.3.252${PLAIN}"
    fi
    echo ""
    echo -e "  3. 一键清理所有实例及虚拟网卡:"
    echo -e "     ${CYAN}${script_name} -clean-all${PLAIN}"
    echo ""
    
    exit 1
}

# === 一键清理功能 ===
clean_all() {
    echo -e "${BOLD}=== 清理所有 FakeConsole 实例 ===${PLAIN}"
    local ns_count=0
    local if_count=0

    # 1. 终止所有 fakeconsole 进程
    if pgrep -f "$BINARY_PATH" >/dev/null 2>&1; then
        pkill -9 -f "$BINARY_PATH" 2>/dev/null
        log_success "已终止所有后台运行的 fakeconsole 进程"
    fi

    # 2. 删除所有特征命名空间 (默认以 fake- 开头)
    for ns in $(ip netns list 2>/dev/null | awk '{print $1}' | grep '^fake-'); do
        # 兼容 OpenWrt: 检查该 ns 内是否有遗留进程并强杀
        PIDS=$(ip netns pids "$ns" 2>/dev/null)
        if [ -n "$PIDS" ]; then
            echo "$PIDS" | xargs kill -9 2>/dev/null
        fi
        ip netns del "$ns" 2>/dev/null
        log_success "已删除网络命名空间: $ns"
        ns_count=$((ns_count+1))
    done

    # 3. 删除残留的虚拟网卡
    for veth in $(ip link show 2>/dev/null | awk -F': ' '{print $2}' | awk -F'@' '{print $1}' | grep '^vh_'); do
        ip link delete "$veth" 2>/dev/null
        log_success "已删除残留虚拟网卡: $veth"
        if_count=$((if_count+1))
    done
    
    # 4. 清理自启项
    if [ -f "/etc/rc.local" ]; then
        SCRIPT_NAME=$(basename "$0")
        if grep -q "$SCRIPT_NAME -ip" /etc/rc.local; then
            # 使用高兼容方法剔除当前脚本的开机自启行
            grep -v "$SCRIPT_NAME -ip" /etc/rc.local > /tmp/rc.local.tmp
            cat /tmp/rc.local.tmp > /etc/rc.local
            rm -f /tmp/rc.local.tmp
            log_success "已清理 /etc/rc.local 中的相关开机自启项"
        fi
    fi

    # 5. 清理日志文件
    rm -f /tmp/fake-*.log 2>/dev/null

    log_info "清理完成！(共移除 $ns_count 个命名空间, $if_count 个虚拟网卡)"
    exit 0
}

# === 写入开机自启功能 ===
add_to_autostart() {
    SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
    SCRIPT_NAME=$(basename "$0")
    FULL_SCRIPT_PATH="$SCRIPT_DIR/$SCRIPT_NAME"
    
    # 构建完整的启动命令，保证重启后参数绝对一致
    START_CMD="$FULL_SCRIPT_PATH -ip $TARGET_IP -type $DEVICE_TYPE -mac $TARGET_MAC -gw $TARGET_GW -name $INSTANCE_NAME"
    
    if [ -f "/etc/rc.local" ]; then
        if grep -Fq "$START_CMD" /etc/rc.local; then
            log_warn "该实例已配置过开机自启，无需重复添加"
        else
            # 兼容所有 Linux/OpenWrt 系统的高可用写入方式
            grep -v "^exit 0" /etc/rc.local > /tmp/rc.local.tmp
            echo "$START_CMD >/dev/null 2>&1" >> /tmp/rc.local.tmp
            echo "sleep 1" >> /tmp/rc.local.tmp
            echo "exit 0" >> /tmp/rc.local.tmp
            cat /tmp/rc.local.tmp > /etc/rc.local
            rm -f /tmp/rc.local.tmp
            chmod +x /etc/rc.local
            log_success "已成功添加开机自启到 /etc/rc.local"
        fi
    else
        log_warn "系统未找到 /etc/rc.local，无法自动配置开机自启"
    fi
}

# === 1. 参数解析 ===
TARGET_IP=""
TARGET_MAC=""
TARGET_GW=""
INSTANCE_NAME=""
DEVICE_TYPE="ps4"

if [ $# -eq 0 ]; then
    log_error "未输入任何参数！"
    print_hint
    exit 1
fi

while [ $# -gt 0 ]; do
    case "$1" in
        -ip)   TARGET_IP="$2"; shift 2 ;;
        -type) 
            if [ "$2" = "ns" ]; then
                DEVICE_TYPE="switch"
            else
                DEVICE_TYPE="$2"
            fi
            shift 2 
            ;;
        -mac)  TARGET_MAC="$2"; shift 2 ;;
        -gw)   TARGET_GW="$2"; shift 2 ;;
        -name) INSTANCE_NAME="$2"; shift 2 ;;
        -autostart|--autostart) AUTO_START=1; shift 1 ;;
        -clean|-clean-all|--clean|--clean-all) clean_all ;;
        -h|--help) show_help ;;
        *) 
            log_error "未知参数 '$1'"
            print_hint
            exit 1 
            ;;
    esac
done

# === 2. 预检与自动补全 ===

echo -e "${BOLD}=== FakeConsole 启动配置程序 ===${PLAIN}"

if [ -z "$TARGET_IP" ]; then
    log_error "未指定 IP 地址 (-ip)！"
    print_hint
    exit 1
fi

HOST_CIDR=$(ip -o -4 addr show "$BRIDGE_IF" 2>/dev/null | awk 'NR==1 {print $4}')
if [ -z "$HOST_CIDR" ]; then
    log_error "网桥 $BRIDGE_IF 不存在或无 IP，请检查网络设置。"
    exit 1
fi
PREFIX="${HOST_CIDR#*/}"

if [ -z "$TARGET_MAC" ]; then
    HEX_SUFFIX=$(echo -n "$TARGET_IP" | md5sum | sed 's/\(..\)/\1:/g' | cut -c 1-8)
    
    if [ "$DEVICE_TYPE" = "switch" ]; then
        IP_LAST_NUM=$(echo "$TARGET_IP" | awk -F. '{print $NF}')
        IDX=$(( IP_LAST_NUM % 5 ))
        case $IDX in
            0) OUI="04:03:D6" ;;
            1) OUI="5C:52:1E" ;;
            2) OUI="60:6B:FF" ;;
            3) OUI="64:B5:C6" ;;
            4) OUI="A4:38:CC" ;;
        esac
    else
        OUI="FC:0F:E6"
    fi
    
    TARGET_MAC="${OUI}:${HEX_SUFFIX}"
else
    TARGET_MAC=$(echo "$TARGET_MAC" | tr 'a-z' 'A-Z')
fi

if [ -z "$TARGET_GW" ]; then
    TARGET_GW=$(ip -4 route show default dev "$BRIDGE_IF" 2>/dev/null | awk '{print $3}' | head -n 1)
    if [ -z "$TARGET_GW" ]; then
         HOST_IP="${HOST_CIDR%/*}"
         TARGET_GW=$(echo "$HOST_IP" | awk -F. '{print $1"."$2"."$3".1"}')
    fi
fi

if [ -z "$INSTANCE_NAME" ]; then
    IP_LAST=$(echo "$TARGET_IP" | awk -F. '{print $NF}')
    INSTANCE_NAME="fake-${DEVICE_TYPE}-${IP_LAST}"
fi

log_info "配置参数已生成:"
print_kv "实例名称" "$INSTANCE_NAME"
print_kv "目标 IP" "$TARGET_IP / $PREFIX"
print_kv "目标 MAC" "$TARGET_MAC"
print_kv "模拟设备" "$DEVICE_TYPE"
echo ""

# === 3. 执行逻辑 ===

if [ ! -f "$BINARY_PATH" ]; then
    log_warn "二进制文件不存在，准备下载..."
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64) SUFFIX="amd64" ;;
        aarch64) SUFFIX="arm64" ;;
        armv7*) SUFFIX="armv7" ;;
        *) log_error "架构 $ARCH 不支持自动下载"; exit 1 ;;
    esac
    
    URL="https://gh-proxy.com/https://github.com/gdfsnhsw/FakeConsole/releases/download/latest/fakeconsole-linux-${SUFFIX}"
    
    if command -v wget >/dev/null; then
        wget --no-check-certificate -q -O "$BINARY_PATH" "$URL"
    else
        curl -L -k -s -o "$BINARY_PATH" "$URL"
    fi
    chmod +x "$BINARY_PATH"
    log_success "下载完成: $BINARY_PATH"
fi

log_info "正在配置隔离网络环境..."

VETH_HASH=$(echo -n "$INSTANCE_NAME" | md5sum | cut -c 1-6)
HOST_VETH="vh_${VETH_HASH}"
CONT_VETH="vc_${VETH_HASH}"

if ip netns list | grep -q "$INSTANCE_NAME"; then
    log_warn "发现旧实例 $INSTANCE_NAME，正在清理..."
    PIDS=$(ip netns pids "$INSTANCE_NAME" 2>/dev/null)
    if [ -n "$PIDS" ]; then
        echo "$PIDS" | xargs kill -9 2>/dev/null
    fi
    ip netns del "$INSTANCE_NAME" 2>/dev/null
fi
ip link delete "$HOST_VETH" 2>/dev/null

ip netns add "$INSTANCE_NAME" >/dev/null 2>&1

ip link add "$HOST_VETH" type veth peer name "$CONT_VETH"
ip link set "$HOST_VETH" master "$BRIDGE_IF"
ip link set "$HOST_VETH" up
ip link set "$CONT_VETH" netns "$INSTANCE_NAME"

ip netns exec "$INSTANCE_NAME" ip link set "$CONT_VETH" name eth0
ip netns exec "$INSTANCE_NAME" ip link set eth0 address "$TARGET_MAC"
ip netns exec "$INSTANCE_NAME" ip addr add "$TARGET_IP/$PREFIX" broadcast + dev eth0
ip netns exec "$INSTANCE_NAME" ip link set eth0 up
ip netns exec "$INSTANCE_NAME" ip route add default via "$TARGET_GW"
ip netns exec "$INSTANCE_NAME" ip link set lo up

log_success "网络环境配置完毕"

LOG_FILE="/tmp/${INSTANCE_NAME}.log"
log_info "正在后台启动进程..."

nohup ip netns exec "$INSTANCE_NAME" "$BINARY_PATH" -type "$DEVICE_TYPE" > "$LOG_FILE" 2>&1 &
PID=$!
sleep 1

# === 4. 处理开机自启 ===
if [ "$AUTO_START" = "1" ]; then
    add_to_autostart
fi

# === 5. 最终状态检查 ===
echo "--------------------------------------------------"
if ps | grep -q "$PID"; then
    echo -e "${GREEN}${BOLD}>>> 启动成功 (SUCCESS) <<<${PLAIN}"
    echo -e "   进程 PID : ${CYAN}$PID${PLAIN}"
    echo -e "   设备类型 : ${CYAN}$DEVICE_TYPE${PLAIN}"
    echo -e "   管理 IP  : ${CYAN}$TARGET_IP${PLAIN}"
    echo -e "   生成 MAC : ${CYAN}$TARGET_MAC${PLAIN}"
    echo -e "   日志文件 : ${YELLOW}$LOG_FILE${PLAIN}"
    if [ "$AUTO_START" = "1" ]; then
        echo -e "   开机自启 : ${GREEN}已配置开启${PLAIN}"
    else
        echo -e "   开机自启 : ${YELLOW}未配置 (可添加 -autostart 参数开启)${PLAIN}"
    fi
    echo -e "   停止命令 : ${CYAN}ip netns del $INSTANCE_NAME${PLAIN}"
    echo -e "   全部清理 : ${CYAN}${script_name} -clean-all${PLAIN}"
else
    echo -e "${RED}${BOLD}>>> 启动失败 (FAILED) <<<${PLAIN}"
    log_error "进程未运行，请检查日志:"
    echo -e "${YELLOW}cat $LOG_FILE${PLAIN}"
fi
echo "--------------------------------------------------"
