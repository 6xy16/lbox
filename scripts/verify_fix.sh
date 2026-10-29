#!/bin/bash
set +e
cd /mnt/d/Lunix

export GO=/home/xy/.local/go/bin/go

echo "=== 重新编译 ==="
make build GO=$GO 2>&1

echo ""
echo "=== 第5步: 内存限制测试 (--mem 64M, stress 申请 256M) ==="
./bin/lbox run --mem 64M ./bin/stress --mode mem --mb 256 --secs 1 2>&1
echo "退出码: $?"
