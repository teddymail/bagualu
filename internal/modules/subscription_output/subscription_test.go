package subscription_output

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/teddymail/bagualu/internal/domain"
)

func TestParseClashPreservesProtocolFields(t *testing.T) {
	input := []byte("proxies:\n  - name: secure\n    type: trojan\n    server: 1.2.3.4\n    port: 443\n    password: secret\n    sni: example.com\n")
	result, err := Parse(input, "https://example.test/sub")
	if err != nil || len(result.Nodes) != 1 {
		t.Fatalf("parse: %v %#v", err, result)
	}
	if result.Nodes[0].RawConfig["sni"] != "example.com" || result.Nodes[0].EndpointIP != "1.2.3.4" {
		t.Fatalf("fields were not preserved: %#v", result.Nodes[0])
	}
}

func TestParseBase64AndRender(t *testing.T) {
	plain := "ss://method:password@example.com:8388#node\n"
	input := []byte(base64.StdEncoding.EncodeToString([]byte(plain)))
	result, err := Parse(input, "source")
	if err != nil || len(result.Nodes) != 1 {
		t.Fatalf("parse: %v %#v", err, result)
	}

	out, err := Render(result.Nodes, RenderOptions{Format: FormatBase64})
	decoded, _ := base64.StdEncoding.DecodeString(string(out))
	if err != nil || string(decoded) != strings.TrimSpace(plain) {
		t.Fatalf("render: %v %q", err, out)
	}
}

func TestParseEncodedShareURIs(t *testing.T) {
	ssPayload := base64.RawStdEncoding.EncodeToString([]byte("chacha20:secret@example.com:443"))
	result, err := Parse([]byte("ss://"+ssPayload+"#ss-node"), "source")
	if err != nil || len(result.Nodes) != 1 || result.Nodes[0].RawConfig["cipher"] != "chacha20" {
		t.Fatalf("ss URI: %v %#v", err, result)
	}
	vmessPayload := base64.StdEncoding.EncodeToString([]byte(`{"v":"2","ps":"vm-node","add":"192.0.2.1","port":"443","id":"uuid"}`))
	result, err = Parse([]byte("vmess://"+vmessPayload), "source")
	if err != nil || len(result.Nodes) != 1 || result.Nodes[0].Name != "vm-node" {
		t.Fatalf("vmess URI: %v %#v", err, result)
	}
}

func TestParseEscapedVLESSRealityURI(t *testing.T) {
	input := `vless\://c4215181-8e81-4eea-b71f-17605c9a8570\@160.16.140.60:443?type=tcp&encryption=none&host=&path=&headerType=none&quicSecurity=none&serviceName=&security=reality&flow=xtls-rprx-vision&fp=chrome&insecure=0&sni=jp.charmnap.com&pbk=CIuFVwMetC8kuCvi7\_buV\_w-HStNrcPfMKpwK-68LGI&sid=8ca03e8a#%E6%97%A5%E6%9C%AC-8%E5%8F%B7`
	result, err := Parse([]byte(input), "manual")
	if err != nil || len(result.Nodes) != 1 {
		t.Fatalf("parse escaped VLESS URI: %v %#v", err, result)
	}
	node := result.Nodes[0]
	if node.Protocol != "vless" || node.Address != "160.16.140.60" || node.Port != 443 || node.Name != "日本-8号" {
		t.Fatalf("unexpected node: %+v", node)
	}
	for key, want := range map[string]string{"security": "reality", "flow": "xtls-rprx-vision", "fp": "chrome", "sni": "jp.charmnap.com", "pbk": "CIuFVwMetC8kuCvi7_buV_w-HStNrcPfMKpwK-68LGI", "sid": "8ca03e8a"} {
		if got := node.RawConfig[key]; got != want {
			t.Errorf("RawConfig[%q] = %v, want %q", key, got, want)
		}
	}
}

func TestRenderClashAndSingBox(t *testing.T) {
	result, err := Parse([]byte("proxies:\n- name: socks\n  type: socks5\n  server: 127.0.0.1\n  port: 1080\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	clash, err := Render(result.Nodes, RenderOptions{Format: FormatClash})
	if err != nil || !strings.Contains(string(clash), "proxy-groups:") {
		t.Fatalf("clash: %v %s", err, clash)
	}
	sing, err := Render(result.Nodes, RenderOptions{Format: FormatSingBox})
	if err != nil || !strings.Contains(string(sing), `"type":"socks"`) {
		t.Fatalf("sing-box: %v %s", err, sing)
	}
}

func TestRenderClashIncludesImportableGroupsAndRules(t *testing.T) {
	nodes := []domain.Node{
		{ID: "us-1", Name: "US", Protocol: "socks5", Address: "192.0.2.1", Port: 1080, Region: "US"},
		{ID: "jp-1", Name: "JP", Protocol: "http", Address: "192.0.2.2", Port: 8080, Region: "JP"},
	}
	out, err := Render(nodes, RenderOptions{Format: FormatClash, TestURL: "https://example.test/ping", TestInterval: 120})
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, expected := range []string{"proxies:", "proxy-groups:", "name: 节点选择", "name: AUTO", "name: US节点", "DIRECT", "rules:", "MATCH,节点选择", "https://example.test/ping", "interval: 120"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("clash output missing %q: %s", expected, text)
		}
	}
}

func TestRenderShareURIsPreservesProtocolsAndParameters(t *testing.T) {
	nodes := []domain.Node{
		{ID: "vless-1", Name: "reality", Protocol: "vless", Address: "192.0.2.3", Port: 443, RawConfig: map[string]any{
			"uuid": "c4215181-8e81-4eea-b71f-17605c9a8570", "security": "reality", "network": "tcp",
			"sni": "example.com", "flow": "xtls-rprx-vision", "fp": "chrome", "pbk": "public-key", "sid": "short",
		}},
		{ID: "vmess-1", Name: "vmess", Protocol: "vmess", Address: "192.0.2.4", Port: 443, RawConfig: map[string]any{
			"uuid": "c4215181-8e81-4eea-b71f-17605c9a8570", "network": "ws", "tls": true, "sni": "example.com", "path": "/edge", "host": "example.com",
		}},
	}
	out, err := Render(nodes, RenderOptions{Format: FormatBase64})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(out))
	if err != nil {
		t.Fatal(err)
	}
	text := string(decoded)
	for _, expected := range []string{"vless://", "security=reality", "pbk=public-key", "vmess://"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("base64 output missing %q: %s", expected, text)
		}
	}
}

func TestPreviewReportsFormatSpecificCompatibility(t *testing.T) {
	nodes := []domain.Node{
		{ID: "valid", Name: "valid", Protocol: "socks5", Address: "192.0.2.5", Port: 1080},
		{ID: "invalid", Name: "invalid", Protocol: "vless", Address: "192.0.2.6", Port: 443, RawConfig: map[string]any{}},
	}
	preview := Preview(nodes, FormatDAE)
	if preview.CompatibleCount != 1 || len(preview.Skipped) != 1 || !strings.Contains(preview.Skipped[0], "dae_unsupported") {
		t.Fatalf("unexpected dae preview: %#v", preview)
	}
	if len(preview.URIs) != 1 || !strings.HasPrefix(preview.URIs[0], "socks5://") {
		t.Fatalf("unexpected uri preview: %#v", preview.URIs)
	}
}

func TestRenderJSONAndOriginal(t *testing.T) {
	result, err := Parse([]byte("ss://method:password@example.com:8388#node\n"), "source")
	if err != nil {
		t.Fatal(err)
	}
	jsonOutput, err := Render(result.Nodes, RenderOptions{Format: FormatJSON})
	if err != nil || !strings.Contains(string(jsonOutput), `"protocol":"ss"`) {
		t.Fatalf("json output: %v %s", err, jsonOutput)
	}
	original, err := Render(result.Nodes, RenderOptions{Format: FormatOriginal})
	if err != nil || !strings.Contains(string(original), "ss://") {
		t.Fatalf("original output: %v %s", err, original)
	}
}

func TestParseSingBoxJSON(t *testing.T) {
	result, err := Parse([]byte(`{"outbounds":[{"type":"socks","tag":"edge","server":"192.0.2.4","server_port":1080}]}`), "source")
	if err != nil || len(result.Nodes) != 1 || result.Nodes[0].Protocol != "socks5" || result.Nodes[0].Name != "edge" {
		t.Fatalf("sing-box parse: %v %#v", err, result)
	}
}

func TestDetectUserAgent(t *testing.T) {
	tests := []struct {
		ua     string
		format Format
	}{
		{"ClashVerge/2", FormatClash}, {"sing-box/1", FormatSingBox},
		{"dae-wing/1", FormatDAE}, {"v2rayNG/1", FormatBase64},
	}
	for _, test := range tests {
		_, got, ambiguous := DetectUserAgent(test.ua)
		if ambiguous || got != test.format {
			t.Errorf("%q => %q ambiguous=%v", test.ua, got, ambiguous)
		}
	}
	if client, _, ambiguous := DetectUserAgent("unknown-client"); client != "unknown" || ambiguous {
		t.Fatalf("unknown UA: %q %v", client, ambiguous)
	}
}
