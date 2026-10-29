#!/bin/bash
set +e
cd /mnt/d/Lunix

echo "=== cgroup-check ==="
make cgroup-check 2>&1

echo ""
echo "=== 第5步: 内存限制测试 (--mem 64M, stress 申请 256M) ==="
./bin/lbox run --mem 64M ./bin/stress --mode mem --mb 256 --secs 1 2>&1
echo "exit=$?"

echo ""
echo "=== 跑完后 lbox 层残留状态 ==="
ls -la /sys/fs/cgroup/lbox/ 2>&1
cat /sys/fs/cgroup/lbox/cgroup.controllers 2>&1
cat /sys/fs/cgroup/lbox/cgroup.subtree_control 2>&1
