#!/bin/bash
# 端到端验证 rootfs 隔离效果
set +e
cd /mnt/d/Lunix

echo "=== 测试 1: 容器内 ls / 应该只看到 rootfs 内容 ==="
./bin/lbox run --rootfs /mnt/d/Lunix/rootfs /bin/sh -c 'ls /'

echo ""
echo "=== 测试 2: 容器内 /bin 下的工具 ==="
./bin/lbox run --rootfs /mnt/d/Lunix/rootfs /bin/sh -c 'ls /bin/ | head -15'

echo ""
echo "=== 测试 3: 验证看不到宿主机文件系统 ==="
./bin/lbox run --rootfs /mnt/d/Lunix/rootfs /bin/sh -c '
echo "ls /home/:"
ls /home/ 2>&1
echo "ls /mnt/:"
ls /mnt/ 2>&1
echo "ls /usr/bin/ | head -3:"
ls /usr/bin/ 2>&1 | head -3
'

echo ""
echo "=== 测试 4: busybox 工具正常工作 ==="
./bin/lbox run --rootfs /mnt/d/Lunix/rootfs /bin/sh -c '
echo "hello from container"
uname -a
hostname
echo "PS1 test:"
echo $PS1
'

echo ""
echo "=== 测试 5: PID 隔离 + rootfs 组合 ==="
./bin/lbox run --rootfs /mnt/d/Lunix/rootfs /bin/sh -c 'echo "容器内进程数:"; ps -e --no-headers | wc -l; echo "容器内进程列表:"; ps -ef'
