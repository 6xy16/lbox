#!/bin/bash
cd /mnt/d/Lunix

# 启动带端口映射的容器
./bin/lbox run --network -p 8080:80 --rootfs /mnt/d/Lunix/rootfs /bin/sleep 3 &
CPID=$!
sleep 1

echo "=== iptables DNAT 规则 ==="
iptables -t nat -L PREROUTING -n | grep 8080

echo ""
echo "=== 等待容器退出 ==="
wait $CPID

echo ""
echo "=== 容器退出后的规则 ==="
iptables -t nat -L PREROUTING -n | grep 8080 || echo "(规则已清除)"
