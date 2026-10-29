#!/bin/bash
cd /mnt/d/Lunix

echo "=== before ==="
cat /var/lib/lbox/assigned_ips

./bin/lbox run --network --rootfs /mnt/d/Lunix/rootfs /bin/sleep 2 &
PID=$!
sleep 1

echo ""
echo "=== after first container ==="
cat /var/lib/lbox/assigned_ips

wait $PID

echo ""
echo "=== after container exited ==="
cat /var/lib/lbox/assigned_ips
