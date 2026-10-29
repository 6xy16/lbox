package runtime

import (
	"strings"
	"syscall"
	"testing"
)

func TestValidateContainerID(t *testing.T) {
	valid := []string{"a", "abc123", "my-container", "test_1.0", "ABC"}
	for _, id := range valid {
		if err := ValidateContainerID(id); err != nil {
			t.Errorf("ValidateContainerID(%q) 应通过，得到 %v", id, err)
		}
	}

	invalid := []string{"", ".", "..", "a/b", "../etc", "a b", "a:b", "..%2f", "x\n", strings.Repeat("a", 65)}
	for _, id := range invalid {
		if err := ValidateContainerID(id); err == nil {
			t.Errorf("ValidateContainerID(%q) 应报错", id)
		}
	}
}

func TestParsePortBinding(t *testing.T) {
	b, err := parsePortBinding("8080:80")
	if err != nil || b.hostPort != 8080 || b.containerPort != 80 {
		t.Fatalf("parsePortBinding(8080:80) = %+v, %v", b, err)
	}
	for _, bad := range []string{"", "8080", "8080:80:90", "0:80", "65536:80", "80:0", "80:65536", "a:b"} {
		if _, err := parsePortBinding(bad); err == nil {
			t.Errorf("parsePortBinding(%q) 应报错", bad)
		}
	}
}

func TestVethNames(t *testing.T) {
	h, c, err := vethNames("10.0.0.2")
	if err != nil || h != "veth-2-h" || c != "veth-2-c" {
		t.Fatalf("vethNames(10.0.0.2) = %q,%q,%v", h, c, err)
	}
	h, c, _ = vethNames("10.0.0.12")
	if h != "veth-12-h" || c != "veth-12-c" {
		t.Fatalf("vethNames(10.0.0.12) 应区分末段 12，得到 %q,%q", h, c)
	}
	if _, _, err := vethNames("10.0.0.2/24"); err == nil {
		t.Error("vethNames 对带掩码的 IP 应报错")
	}
}

func TestWaitStatusExitCode(t *testing.T) {
	if code := waitStatusExitCode(0); code != 0 {
		t.Errorf("正常退出码应为 0，得到 %d", code)
	}
	exited := syscall.WaitStatus(7 << 8)
	if code := waitStatusExitCode(exited); code != 7 {
		t.Errorf("退出码 7 应透传，得到 %d", code)
	}
	signaled := syscall.WaitStatus(0x80 | 9) // SIGKILL(9)
	if code := waitStatusExitCode(signaled); code != 137 {
		t.Errorf("被 SIGKILL 终止应得到 137，得到 %d", code)
	}
}

func TestContains(t *testing.T) {
	if !contains([]string{"cpu", "memory"}, "cpu") {
		t.Error("contains 应返回 true")
	}
	if contains([]string{"cpu", "memory"}, "pids") {
		t.Error("contains 应返回 false")
	}
}
