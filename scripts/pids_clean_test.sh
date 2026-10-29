#!/bin/bash
set +e

echo "=== 干净测试：shell 先进入 cgroup，再 fork ==="

rmdir /sys/fs/cgroup/lbox/test-pids 2>/dev/null
mkdir -p /sys/fs/cgroup/lbox/test-pids
echo "8" > /sys/fs/cgroup/lbox/test-pids/pids.max

cd /sys/fs/cgroup/lbox/test-pids
sh -c '
echo $$ > cgroup.procs
echo "my cgroup: $(cat /proc/self/cgroup)"
echo "pids.current: $(cat pids.current)"
echo "--- fork 10 个 sleep ---"
for i in 1 2 3 4 5 6 7 8 9 10; do
    sleep 5 &
    echo "  fork #$i, pid=$!"
done
sleep 1
echo "pids.current: $(cat pids.current)"
echo "存活进程数: $(ps -e --no-headers | wc -l)"
'

echo ""
echo "=== 对照：不进 cgroup，直接 fork ==="
sh -c '
echo "--- fork 10 个 sleep（不进 cgroup）---"
for i in 1 2 3 4 5 6 7 8 9 10; do
    sleep 5 &
done
sleep 1
echo "存活进程数: $(ps -e --no-headers | wc -l)"
'

# 看看内核版本
echo ""
echo "=== 内核版本 ==="
uname -r
cat /proc/version
