package mihomo

import (
	"strings"
	"testing"

	"github.com/teddymail/bagualu/internal/domain"
)

func TestConfigPreservesRawProtocolFieldsDeterministically(t *testing.T) {
	nodes := []domain.Node{{
		ID: "node-1", Name: "node", Protocol: "ss", Address: "127.0.0.1", Port: 443,
		RawConfig: map[string]any{"password": "secret", "cipher": " aes-256-gcm", "tls": true},
	}}
	first, digest, err := Config(nodes, "127.0.0.1:9090", "token", 7890)
	if err != nil {
		t.Fatal(err)
	}
	second, secondDigest, err := Config(nodes, "127.0.0.1:9090", "token", 7890)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || digest != secondDigest {
		t.Fatal("configuration or digest is not deterministic")
	}
	for _, field := range []string{`password: "secret"`, `cipher: " aes-256-gcm"`, "tls: true"} {
		if !strings.Contains(string(first), field) {
			t.Fatalf("missing raw field %q in config:\n%s", field, first)
		}
	}
}

func TestConfigRejectsInvalidNode(t *testing.T) {
	_, _, err := Config([]domain.Node{{ID: "bad", Name: "missing-port"}}, "127.0.0.1:9090", "", 7890)
	if err == nil {
		t.Fatal("expected invalid node error")
	}
}

func TestConfigNormalizesVLESSURIFields(t *testing.T) {
	node := domain.Node{
		ID: "vless-node", Name: "reality", Protocol: "vless", Address: "192.0.2.1", Port: 443,
		RawConfig: map[string]any{
			"username": "uuid-value", "type": "tcp", "security": "reality", "pbk": "public-key",
			"sid": "short-id", "sni": "example.com", "fp": "chrome", "insecure": "0",
		},
	}
	config, _, err := Config([]domain.Node{node}, "127.0.0.1:9090", "token", 7890)
	if err != nil {
		t.Fatal(err)
	}
	text := string(config)
	for _, field := range []string{
		`uuid: "uuid-value"`, `network: "tcp"`, `servername: "example.com"`,
		`client-fingerprint: "chrome"`, `skip-cert-verify: false`, `tls: true`,
		`reality-opts: {"public-key":"public-key","short-id":"short-id"}`,
	} {
		if !strings.Contains(text, field) {
			t.Fatalf("missing normalized field %q in config:\n%s", field, text)
		}
	}
	for _, field := range []string{"username:", "pbk:", "sid:", "security:", "sni:", "insecure:"} {
		if strings.Contains(text, field) {
			t.Fatalf("legacy field %q should not be emitted in config:\n%s", field, text)
		}
	}
}

func TestProxyNameIsStableAndUnique(t *testing.T) {
	first := ProxyName(domain.Node{ID: "node-123456789", Name: "Japan", Protocol: "vless", Address: "192.0.2.1"})
	second := ProxyName(domain.Node{ID: "node-abcdefghi", Name: "Japan", Protocol: "vless", Address: "192.0.2.2"})
	if first == second || first != "Japan [node-123]" || second != "Japan [node-abc]" {
		t.Fatalf("unexpected proxy names: %q and %q", first, second)
	}
}
