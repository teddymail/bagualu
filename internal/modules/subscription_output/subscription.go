// Package subscription_output parses proxy subscriptions and renders client
// consumable resources. It deliberately has no transport or persistence code.
package subscription_output

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/teddymail/bagualu/internal/domain"
	"gopkg.in/yaml.v3"
)

type Format string

const (
	FormatClash    Format = "clash"
	FormatBase64   Format = "base64"
	FormatSingBox  Format = "sing-box"
	FormatDAE      Format = "dae"
	FormatDAED     Format = "daed"
	FormatJSON     Format = "json"
	FormatOriginal Format = "original"
)

var (
	ErrInvalidSubscription = errors.New("invalid_subscription")
	ErrNoCompatibleNodes   = errors.New("subscription_no_compatible_nodes")
)

type Result struct {
	Nodes   []domain.Node
	Skipped []string
}

// Parse accepts Clash/Mihomo YAML, a URI list, or a (possibly padded) Base64
// encoded URI list. It never returns partially parsed nodes on malformed YAML.
func Parse(input []byte, source string) (Result, error) {
	data := bytes.TrimSpace(input)
	if len(data) == 0 {
		return Result{}, fmt.Errorf("%w: empty input", ErrInvalidSubscription)
	}
	if bytes.HasPrefix(data, []byte("{")) {
		if result, ok := parseSingBox(data, source); ok {
			if len(result.Nodes) == 0 {
				return result, ErrNoCompatibleNodes
			}
			return result, nil
		}
	}
	if looksLikeYAML(data) {
		return parseYAML(data, source)
	}
	if decoded, ok := decodeSubscription(data); ok {
		data = decoded
	}
	return parseURIs(data, source)
}

func parseSingBox(data []byte, source string) (Result, bool) {
	var document struct {
		Outbounds []map[string]any `json:"outbounds"`
	}
	if json.Unmarshal(data, &document) != nil || len(document.Outbounds) == 0 {
		return Result{}, false
	}
	result := Result{Nodes: make([]domain.Node, 0, len(document.Outbounds))}
	for index, outbound := range document.Outbounds {
		raw := cloneMap(outbound)
		protocol := strings.ToLower(strings.TrimSpace(fmt.Sprint(raw["type"])))
		switch protocol {
		case "shadowsocks":
			protocol = "ss"
		case "socks":
			protocol = "socks5"
		}
		raw["type"] = protocol
		if _, ok := raw["name"]; !ok {
			raw["name"] = raw["tag"]
		}
		if _, ok := raw["server"]; !ok {
			raw["server"] = raw["address"]
		}
		if port, ok := raw["server_port"]; ok {
			raw["port"] = port
		}
		delete(raw, "tag")
		delete(raw, "server_port")
		node, err := nodeFromMap(raw, source, index)
		if err != nil {
			result.Skipped = append(result.Skipped, "singbox_unsupported: "+err.Error())
			continue
		}
		result.Nodes = append(result.Nodes, node)
	}
	if len(result.Nodes) == 0 {
		return result, true
	}
	return result, true
}

func ParseSubscription(input []byte, source string) (Result, error) { return Parse(input, source) }

func looksLikeYAML(data []byte) bool {
	return bytes.Contains(data, []byte("proxies:")) || bytes.HasPrefix(data, []byte("---")) ||
		bytes.HasPrefix(data, []byte("{")) || bytes.Contains(data, []byte("\nproxies:"))
}

func parseYAML(data []byte, source string) (Result, error) {
	var doc struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Result{}, fmt.Errorf("%w: yaml: %v", ErrInvalidSubscription, err)
	}
	if len(doc.Proxies) == 0 {
		return Result{}, fmt.Errorf("%w: missing proxies", ErrInvalidSubscription)
	}
	result := Result{Nodes: make([]domain.Node, 0, len(doc.Proxies))}
	for i, raw := range doc.Proxies {
		node, err := nodeFromMap(raw, source, i)
		if err != nil {
			result.Skipped = append(result.Skipped, "clash_unsupported: "+err.Error())
			continue
		}
		result.Nodes = append(result.Nodes, node)
	}
	if len(result.Nodes) == 0 {
		return result, fmt.Errorf("%w: no compatible nodes", ErrNoCompatibleNodes)
	}
	return result, nil
}

func parseURIs(data []byte, source string) (Result, error) {
	result := Result{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		node, err := nodeFromURI(line, source, len(result.Nodes))
		if err != nil {
			result.Skipped = append(result.Skipped, "uri_unsupported: "+err.Error())
			continue
		}
		result.Nodes = append(result.Nodes, node)
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("%w: %v", ErrInvalidSubscription, err)
	}
	if len(result.Nodes) == 0 {
		return result, fmt.Errorf("%w: no compatible nodes", ErrNoCompatibleNodes)
	}
	return result, nil
}

func decodeSubscription(data []byte) ([]byte, bool) {
	compact := bytes.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, data)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := enc.DecodeString(string(compact)); err == nil &&
			(bytes.Contains(decoded, []byte("://")) || bytes.Contains(decoded, []byte("ss://"))) {
			return decoded, true
		}
	}
	return data, false
}

func nodeFromMap(raw map[string]any, source string, index int) (domain.Node, error) {
	get := func(key string) any { return raw[key] }
	protocol := strings.ToLower(strings.TrimSpace(fmt.Sprint(get("type"))))
	address := strings.TrimSpace(fmt.Sprint(get("server")))
	if protocol == "" || address == "" {
		return domain.Node{}, errors.New("type and server are required")
	}
	port, err := intValue(get("port"))
	if err != nil || port < 1 || port > 65535 {
		return domain.Node{}, errors.New("invalid port")
	}
	name := strings.TrimSpace(fmt.Sprint(get("name")))
	if name == "" {
		name = protocol + "-" + address
	}
	config := cloneMap(raw)
	return domain.Node{
		ID: stableID(protocol, address, port, config), Name: name, Protocol: protocol,
		Address: address, EndpointIP: endpointIP(address), Port: port,
		SourceURL: source, Status: domain.NodeActive, RawConfig: config,
	}, nil
}

func nodeFromURI(rawURI, source string, index int) (domain.Node, error) {
	rawURI = normalizeNodeURI(rawURI)
	schemeEnd := strings.Index(rawURI, "://")
	if schemeEnd > 0 {
		scheme := strings.ToLower(rawURI[:schemeEnd])
		switch scheme {
		case "ss":
			rawURI = decodeSSURI(rawURI)
		case "vmess":
			if node, ok := decodeVMessURI(rawURI, source); ok {
				return node, nil
			}
		}
	}
	u, err := url.Parse(rawURI)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return domain.Node{}, errors.New("invalid URI")
	}
	protocol := strings.ToLower(u.Scheme)
	if protocol == "hysteria2" {
		protocol = "hysteria2"
	}
	if protocol != "ss" && protocol != "ssr" && protocol != "vmess" && protocol != "vless" &&
		protocol != "trojan" && protocol != "socks5" && protocol != "socks" && protocol != "http" &&
		protocol != "hysteria2" && protocol != "tuic" {
		return domain.Node{}, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return domain.Node{}, errors.New("invalid port")
	}
	name, _ := url.QueryUnescape(u.Fragment)
	if name == "" {
		name = protocol + "-" + u.Hostname()
	}
	config := map[string]any{"type": protocol, "server": u.Hostname(), "port": port, "name": name, "uri": rawURI}
	if u.User != nil {
		config["username"] = u.User.Username()
		if password, ok := u.User.Password(); ok {
			config["password"] = password
		}
		if protocol == "ss" {
			config["cipher"] = u.User.Username()
		}
	}
	for key, values := range u.Query() {
		if len(values) > 0 {
			if key == "type" {
				config["network"] = values[0]
				continue
			}
			if key == "server" || key == "port" || key == "name" {
				continue
			}
			config[key] = values[0]
		}
	}
	return domain.Node{
		ID: stableID(protocol, u.Hostname(), port, config), Name: name, Protocol: protocol,
		Address: u.Hostname(), EndpointIP: endpointIP(u.Hostname()), Port: port,
		SourceURL: source, Status: domain.NodeActive, RawConfig: config,
	}, nil
}

func normalizeNodeURI(rawURI string) string {
	rawURI = strings.TrimSpace(rawURI)
	rawURI = strings.Trim(rawURI, "\"'")
	for _, pair := range []struct{ escaped, plain string }{
		{`\\://`, `://`}, {`\://`, `://`}, {`\@`, `@`}, {`\_`, `_`},
	} {
		rawURI = strings.ReplaceAll(rawURI, pair.escaped, pair.plain)
	}
	return strings.TrimSpace(rawURI)
}

func decodeSSURI(raw string) string {
	body := raw[len("ss://"):]
	fragment := ""
	if at := strings.IndexByte(body, '#'); at >= 0 {
		fragment, body = body[at:], body[:at]
	}
	if strings.Contains(body, "@") {
		return "ss://" + body + fragment
	}
	compact := strings.Map(func(r rune) rune {
		if r == '-' {
			return '+'
		}
		if r == '_' {
			return '/'
		}
		return r
	}, body)
	for _, encoding := range []*base64.Encoding{
		base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding,
	} {
		if decoded, err := encoding.DecodeString(compact); err == nil && bytes.Contains(decoded, []byte("@")) {
			return "ss://" + string(decoded) + fragment
		}
	}
	return raw
}

func decodeVMessURI(raw string, source string) (domain.Node, bool) {
	body := raw[len("vmess://"):]
	if at := strings.IndexByte(body, '#'); at >= 0 {
		body = body[:at]
	}
	decoded, ok := decodeBase64Text(body)
	if !ok {
		return domain.Node{}, false
	}
	var config map[string]any
	if json.Unmarshal([]byte(decoded), &config) != nil {
		return domain.Node{}, false
	}
	address := strings.TrimSpace(fmt.Sprint(config["add"]))
	port, err := intValue(config["port"])
	if address == "" || err != nil || port < 1 || port > 65535 {
		return domain.Node{}, false
	}
	name := strings.TrimSpace(fmt.Sprint(config["ps"]))
	if name == "" {
		name = "vmess-" + address
	}
	config["type"], config["server"], config["port"], config["name"], config["uri"] =
		"vmess", address, port, name, raw
	return domain.Node{
		ID: stableID("vmess", address, port, config), Name: name, Protocol: "vmess",
		Address: address, EndpointIP: endpointIP(address), Port: port,
		SourceURL: source, Status: domain.NodeActive, RawConfig: config,
	}, true
}

func decodeBase64Text(value string) (string, bool) {
	value = strings.TrimSpace(value)
	for _, encoding := range []*base64.Encoding{
		base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding,
	} {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return string(decoded), true
		}
	}
	return "", false
}

func intValue(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case uint64:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	default:
		return 0, errors.New("not an integer")
	}
}

func endpointIP(address string) string {
	if ip := net.ParseIP(strings.Trim(address, "[]")); ip != nil {
		return ip.String()
	}
	return ""
}

func stableID(protocol, address string, port int, config map[string]any) string {
	identity := cloneMap(config)
	// Display names and original URI text are source metadata, not node
	// identity. This lets identical nodes from different subscriptions merge.
	delete(identity, "name")
	delete(identity, "uri")
	b, _ := json.Marshal(identity)
	h := domainHash([]byte(fmt.Sprintf("%s|%s|%d|%s", protocol, address, port, b)))
	return hex.EncodeToString(h[:8])
}

func domainHash(data []byte) [32]byte {
	var out [32]byte
	for i, b := range data {
		out[i%32] = out[i%32]*31 + b
	}
	return out
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// DetectUserAgent chooses only a representation; it has no authorization role.
func DetectUserAgent(ua string) (client string, format Format, ambiguous bool) {
	type rule struct {
		client  string
		format  Format
		pattern string
	}
	rules := []rule{
		{"sing-box", FormatSingBox, `sing-box|(^|[^a-z])sfa([^a-z]|$)|(^|[^a-z])sfi([^a-z]|$)|(^|[^a-z])sfm([^a-z]|$)`},
		{"dae", FormatDAE, `dae-wing|daed|(^|[^a-z])dae([^a-z]|$)`},
		{"base64", FormatBase64, `v2rayng|v2rayn|passwall|shadowsocksr|shadowrocket|nekobox`},
		{"clash", FormatClash, `clashverge|clash verge|mihomo|clash\.meta|openclash|clashforandroid|clashx|nikki|(^|[^a-z])clash([^a-z]|$)`},
	}
	lower := strings.ToLower(ua)
	var matches []rule
	for _, r := range rules {
		if regexp.MustCompile(r.pattern).MatchString(lower) {
			matches = append(matches, r)
		}
	}
	if len(matches) == 1 {
		return matches[0].client, matches[0].format, false
	}
	if len(matches) > 1 {
		return "ambiguous", "", true
	}
	return "unknown", "", false
}
