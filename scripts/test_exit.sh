#!/bin/bash
cd /mnt/d/Lunix
{
echo "=== 测试退出码 42 ==="
./bin/lbox run --rootfs /mnt/d/Lunix/rootfs /bin/sh -c "exit 42"
echo "lbox exit=$?"
echo ""
echo "=== 测试正常退出码 0 ==="
./bin/lbox run --rootfs /mnt/d/Lunix/rootfs /bin/echo "hello world"
echo "lbox exit=$?"
} > /tmp/lbox_test_output.txt 2>&1
cat /tmp/lbox_test_output.txt
