#!/bin/bash
rm -rf /var/lib/lbox
cd /mnt/d/Lunix

echo "=== before ==="
cat /var/lib/lbox/assigned_ips 2>/dev/null || echo "(empty)"

echo ""
echo "=== run container ==="
./bin/lbox run --network --rootfs /mnt/d/Lunix/rootfs /bin/sleep 1

echo ""
echo "=== after ==="
cat /var/lib/lbox/assigned_ips 2>/dev/null || echo "(empty)"
