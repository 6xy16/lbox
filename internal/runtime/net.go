package runtime

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

// 网络配置常量
const (
	BridgeName      = "lbox0"       // bridge 网卡名
	BridgeIP        = "10.0.0.1/24" // bridge IP
	BridgeGateway   = "10.0.0.1"
	ContainerIPBase = "10.0.0." // 容器 IP 前缀
)

// portBinding 已解析的端口映射。
type portBinding struct {
	hostPort      int
	containerPort int
}

// parsePortBinding 校验并解析 "hostPort:containerPort"。
func parsePortBinding(raw string) (portBinding, error) {
	var b portBinding
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 {
		return b, fmt.Errorf("端口映射格式错误: %q（应为 宿主端口:容器端口）", raw)
	}
	hp, err1 := strconv.Atoi(parts[0])
	cp, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return b, fmt.Errorf("端口映射格式错误: %q", raw)
	}
	if hp < 1 || hp > 65535 || cp < 1 || cp > 65535 {
		return b, fmt.Errorf("端口映射端口号超出范围 1~65535: %q", raw)
	}
	b.hostPort = hp
	b.containerPort = cp
	return b, nil
}

// vethNames 根据容器 IP 生成 veth 网卡名；用末段整数而非末位字符，避免 10.0.0.2 与 10.0.0.12 碰撞。
func vethNames(containerIP string) (host, container string, err error) {
	ip := net.ParseIP(containerIP)
	if ip == nil {
		return "", "", fmt.Errorf("无效容器 IP %q", containerIP)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return "", "", fmt.Errorf("容器 IP %q 不是 IPv4", containerIP)
	}
	host = fmt.Sprintf("veth-%d-h", int(ip4[3]))
	container = fmt.Sprintf("veth-%d-c", int(ip4[3]))
	return host, container, nil
}

// SetupNetwork 为容器配置网络：bridge + veth + NAT。
func SetupNetwork(containerPID int, containerIP string) error {
	if err := ensureBridge(); err != nil {
		return fmt.Errorf("创建 bridge 失败: %w", err)
	}

	vethHost, vethContainer, err := vethNames(containerIP)
	if err != nil {
		return err
	}
	if err := createVethPair(vethHost, vethContainer); err != nil {
		return fmt.Errorf("创建 veth pair 失败: %w", err)
	}
	if err := attachToBridge(vethHost, BridgeName); err != nil {
		return fmt.Errorf("veth host 端挂到 bridge 失败: %w", err)
	}
	if err := moveVethToContainer(vethContainer, containerPID); err != nil {
		return fmt.Errorf("移动 veth 到容器 namespace 失败: %w", err)
	}
	if err := setupNAT(); err != nil {
		return fmt.Errorf("配置 NAT 失败: %w", err)
	}
	return nil
}

func ensureBridge() error {
	if err := run("ip", "link", "show", BridgeName); err == nil {
		return run("ip", "link", "set", BridgeName, "up")
	}
	if err := run("ip", "link", "add", BridgeName, "type", "bridge"); err != nil {
		return err
	}
	if err := run("ip", "addr", "add", BridgeIP, "dev", BridgeName); err != nil {
		return err
	}
	return run("ip", "link", "set", BridgeName, "up")
}

func createVethPair(host, container string) error {
	return run("ip", "link", "add", host, "type", "veth", "peer", "name", container)
}

func attachToBridge(veth, bridge string) error {
	if err := run("ip", "link", "set", veth, "master", bridge); err != nil {
		return err
	}
	return run("ip", "link", "set", veth, "up")
}

func moveVethToContainer(veth string, pid int) error {
	return run("ip", "link", "set", veth, "netns", strconv.Itoa(pid))
}

// iptablesRule 描述一条要幂等维护的 iptables 规则。
type iptablesRule struct {
	table string
	chain string
	args  []string
}

// ruleExists 通过 -C 判断规则是否已存在；存在时返回 true。
func ruleExists(table, chain string, args ...string) bool {
	checkArgs := append([]string{"-t", table, "-C", chain}, args...)
	return run("iptables", checkArgs...) == nil
}

// appendRule 只在规则不存在时追加，避免重复堆积。
func appendRule(table, chain string, args ...string) error {
	if ruleExists(table, chain, args...) {
		return nil
	}
	appendArgs := append([]string{"-t", table, "-A", chain}, args...)
	return run("iptables", appendArgs...)
}

func setupNAT() error {
	if err := run("sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		return err
	}
	if err := appendRule("nat", "POSTROUTING", "-s", "10.0.0.0/24", "!", "-d", "10.0.0.0/24", "-j", "MASQUERADE"); err != nil {
		return err
	}
	if err := appendRule("filter", "FORWARD", "-i", BridgeName, "-j", "ACCEPT"); err != nil {
		return err
	}
	return appendRule("filter", "FORWARD", "-o", BridgeName, "-j", "ACCEPT")
}

func dnats(containerIP string, hostPort, containerPort int) [][]string {
	target := fmt.Sprintf("%s:%d", containerIP, containerPort)
	dport := strconv.Itoa(hostPort)
	return [][]string{
		{"-p", "tcp", "--dport", dport, "-j", "DNAT", "--to-destination", target},
		{"-p", "udp", "--dport", dport, "-j", "DNAT", "--to-destination", target},
	}
}

func SetupPortForward(containerIP string, hostPort, containerPort int) error {
	for _, args := range dnats(containerIP, hostPort, containerPort) {
		if err := appendRule("nat", "PREROUTING", args...); err != nil {
			return err
		}
		if err := appendRule("nat", "OUTPUT", args...); err != nil {
			return err
		}
	}
	return nil
}

func TeardownPortForward(containerIP string, hostPort, containerPort int) error {
	for _, chain := range []string{"PREROUTING", "OUTPUT"} {
		for _, args := range dnats(containerIP, hostPort, containerPort) {
			deleteArgs := append([]string{"-t", "nat", "-D", chain}, args...)
			_ = run("iptables", deleteArgs...) // 不存在或已删除都视为成功
		}
	}
	return nil
}

// run 执行一个命令，失败时携带 stderr 便于定位。
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
