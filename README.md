# FakeConsole
必选参数:
  -ip <IP>        容器内部 IP (例: 192.168.1.2)

可选参数:
  -type <Type>    设备类型: ps4(默认)、steamdeck、switch(或简写ns)
  -mac <MAC>      手动 MAC (默认根据 IP 自动生成)
  -gw <GW>        手动网关 (默认自动获取)
  -name <Name>    手动实例名
  -autostart      添加到 /etc/rc.local 实现开机自启
  -clean-all      清理并删除所有已创建的实例和虚拟网卡 (简写 -clean)
  -h, --help      显示帮助
