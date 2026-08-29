package mihomo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/teddymail/bagualu/internal/domain"
)

// Config renders only the public Mihomo configuration boundary. Protocol-specific
// fields remain in RawConfig and are not interpreted by Bagualu.
func Config(nodes []domain.Node, controller, secret string, proxyPort int) ([]byte, string, error) {
	if controller == "" || proxyPort < 1 || proxyPort > 65535 {
		return nil, "", fmt.Errorf("invalid mihomo listener configuration")
	}
	sorted := append([]domain.Node(nil), nodes...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	var output strings.Builder
	output.WriteString("allow-lan: false\n")
	output.WriteString("mode: rule\n")
	output.WriteString("external-controller: " + controller + "\n")
	output.WriteString("secret: " + secret + "\n")
	output.WriteString("mixed-port: " + strconv.Itoa(proxyPort) + "\n")
	output.WriteString("proxies:\n")
	names := make([]string, 0, len(sorted))
	for _, node := range sorted {
		if node.Name == "" || node.Protocol == "" || node.Address == "" || node.Port < 1 {
			return nil, "", fmt.Errorf("invalid mihomo node %q", node.ID)
		}
		proxyName := ProxyName(node)
		output.WriteString("  - name: " + yamlString(proxyName) + "\n")
		output.WriteString("    type: " + yamlString(node.Protocol) + "\n")
		output.WriteString("    server: " + yamlString(node.Address) + "\n")
		output.WriteString("    port: " + strconv.Itoa(node.Port) + "\n")
		config := normalizeNodeConfig(node)
		keys := make([]string, 0, len(config))
		for key := range config {
			if key != "name" && key != "type" && key != "server" && key != "port" {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			value, err := json.Marshal(config[key])
			if err != nil {
				return nil, "", fmt.Errorf("invalid mihomo node %q field %q: %w", node.ID, key, err)
			}
			output.WriteString("    " + yamlKey(key) + ": " + string(value) + "\n")
		}
		names = append(names, proxyName)
	}
	output.WriteString("proxy-groups:\n")
	output.WriteString("  - name: \"Bagualu-Test\"\n")
	output.WriteString("    type: select\n")
	output.WriteString("    proxies:\n")
	if len(names) == 0 {
		output.WriteString("      - DIRECT\n")
	}
	for _, name := range names {
		output.WriteString("      - " + yamlString(name) + "\n")
	}
	output.WriteString("rules:\n")
	output.WriteString("  - MATCH,Bagualu-Test\n")
	data := []byte(output.String())
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:]), nil
}

func ProxyName(node domain.Node) string {
	name := strings.TrimSpace(node.Name)
	if name == "" {
		name = node.Protocol + "-" + node.Address
	}
	identifier := strings.TrimSpace(node.ID)
	if len(identifier) > 8 {
		identifier = identifier[:8]
	}
	if identifier == "" {
		return name
	}
	return name + " [" + identifier + "]"
}

func normalizeNodeConfig(node domain.Node) map[string]any {
	config := cloneMap(node.RawConfig)
	deleteKeys(config, "name", "server", "port", "uri")

	switch strings.ToLower(node.Protocol) {
	case "vless":
		normalizeVLESSConfig(config)
	case "vmess":
		normalizeVMessConfig(config)
	case "trojan", "hysteria2":
		if _, ok := config["password"]; !ok {
			if username, valid := config["username"].(string); valid && username != "" {
				config["password"] = username
			}
		}
		delete(config, "username")
	}

	return config
}

func normalizeVLESSConfig(config map[string]any) {
	if _, ok := config["uuid"]; !ok {
		if username, valid := config["username"].(string); valid && username != "" {
			config["uuid"] = username
		}
	}
	if _, ok := config["servername"]; !ok {
		if sni, valid := config["sni"].(string); valid && sni != "" {
			config["servername"] = sni
		}
	}
	if _, ok := config["client-fingerprint"]; !ok {
		if fingerprint, valid := config["fp"].(string); valid && fingerprint != "" {
			config["client-fingerprint"] = fingerprint
		}
	}
	if _, ok := config["skip-cert-verify"]; !ok {
		if insecure, valid := booleanValue(config["insecure"]); valid {
			config["skip-cert-verify"] = insecure
		}
	}
	if _, ok := config["network"]; !ok {
		if network, valid := config["transport"].(string); valid && network != "" {
			config["network"] = network
		} else if network, valid := config["type"].(string); valid && network != "" {
			config["network"] = network
		}
	}
	if security, valid := config["security"].(string); valid {
		if security == "tls" || security == "reality" {
			config["tls"] = true
		}
		if security == "reality" {
			reality := map[string]any{}
			if existing, valid := config["reality-opts"].(map[string]any); valid {
				reality = cloneMap(existing)
			}
			if publicKey, valid := config["pbk"].(string); valid && publicKey != "" {
				reality["public-key"] = publicKey
			}
			if shortID, valid := config["sid"].(string); valid && shortID != "" {
				reality["short-id"] = shortID
			}
			if len(reality) > 0 {
				config["reality-opts"] = reality
			}
		}
	}
	if network, _ := config["network"].(string); network == "ws" {
		wsOptions := map[string]any{}
		if existing, valid := config["ws-opts"].(map[string]any); valid {
			wsOptions = cloneMap(existing)
		}
		if path, valid := config["path"].(string); valid && path != "" {
			wsOptions["path"] = path
		}
		if host, valid := config["host"].(string); valid && host != "" {
			wsOptions["headers"] = map[string]any{"Host": host}
		}
		if len(wsOptions) > 0 {
			config["ws-opts"] = wsOptions
		}
	}
	deleteKeys(config, "username", "security", "sni", "fp", "insecure", "pbk", "sid", "transport", "type", "path", "host", "headerType", "quicSecurity", "serviceName")
}

func normalizeVMessConfig(config map[string]any) {
	if _, ok := config["uuid"]; !ok {
		if id, valid := config["id"].(string); valid && id != "" {
			config["uuid"] = id
		}
	}
	if _, ok := config["network"]; !ok {
		if network, valid := config["net"].(string); valid && network != "" {
			config["network"] = network
		}
	}
	if tls, valid := config["tls"].(string); valid {
		config["tls"] = tls != "" && tls != "none" && tls != "false" && tls != "0"
	}
	if network, _ := config["network"].(string); network == "ws" {
		wsOptions := map[string]any{}
		if path, valid := config["path"].(string); valid && path != "" {
			wsOptions["path"] = path
		}
		if host, valid := config["host"].(string); valid && host != "" {
			wsOptions["headers"] = map[string]any{"Host": host}
		}
		if len(wsOptions) > 0 {
			config["ws-opts"] = wsOptions
		}
	}
	deleteKeys(config, "id", "add", "net", "host", "path")
}

func booleanValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true, true
		case "0", "false", "no", "off":
			return false, true
		}
	}
	return false, false
}

func yamlKey(value string) string {
	if value == "" {
		return strconv.Quote(value)
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return strconv.Quote(value)
		}
	}
	return value
}

func yamlString(value string) string {
	return strconv.Quote(value)
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func deleteKeys(input map[string]any, keys ...string) {
	for _, key := range keys {
		delete(input, key)
	}
}
