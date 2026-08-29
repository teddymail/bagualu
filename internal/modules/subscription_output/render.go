package subscription_output

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/teddymail/bagualu/internal/domain"
	"gopkg.in/yaml.v3"
)

type RenderOptions struct {
	Format       Format
	TestURL      string
	TestInterval int
}

type PreviewResult struct {
	Format          Format   `json:"format"`
	CompatibleCount int      `json:"compatible_count"`
	Skipped         []string `json:"skipped"`
	Regions         []string `json:"regions"`
	URIs            []string `json:"uris,omitempty"`
}

// Render produces a complete payload. Callers should filter nodes with the
// resource module before rendering.
func Render(nodes []domain.Node, options RenderOptions) ([]byte, error) {
	if len(nodes) == 0 {
		return nil, ErrNoCompatibleNodes
	}
	switch options.Format {
	case FormatClash:
		return renderClash(nodes, options)
	case FormatBase64, FormatDAE, FormatDAED:
		return renderBase64(nodes)
	case FormatSingBox:
		return renderSingBox(nodes)
	case FormatJSON:
		return renderJSON(nodes)
	case FormatOriginal:
		return renderOriginal(nodes)
	default:
		return nil, fmt.Errorf("unsupported_subscription_format: %q", options.Format)
	}
}

func Preview(nodes []domain.Node, format Format) PreviewResult {
	result := PreviewResult{Format: format, Skipped: []string{}, Regions: []string{}}
	regions := map[string]bool{}
	for _, node := range nodes {
		if err := compatibleNode(node, format); err != nil {
			result.Skipped = append(result.Skipped, previewReason(node, format, err))
			continue
		}
		result.CompatibleCount++
		if node.Region != "" {
			regions[node.Region] = true
		}
		if format == FormatBase64 || format == FormatDAE || format == FormatDAED || format == FormatOriginal {
			if uri := nodeShareURI(node); uri != "" {
				result.URIs = append(result.URIs, uri)
			}
		}
	}
	for region := range regions {
		result.Regions = append(result.Regions, region)
	}
	sort.Strings(result.Regions)
	return result
}

func compatibleNode(node domain.Node, format Format) error {
	switch format {
	case FormatClash:
		if node.Protocol == "" || node.Address == "" || node.Port < 1 {
			return errors.New("missing type, server or port")
		}
	case FormatBase64, FormatDAE, FormatDAED, FormatOriginal:
		if nodeShareURI(node) == "" {
			return errors.New("no standard share URI")
		}
	case FormatSingBox:
		_, err := singBoxOutbound(node)
		return err
	case FormatJSON:
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
	return nil
}

func previewReason(node domain.Node, format Format, err error) string {
	prefix := string(format) + "_unsupported"
	if format == FormatClash || format == FormatJSON {
		prefix = string(format) + "_invalid"
	}
	return fmt.Sprintf("%s: %s: %v", node.ID, prefix, err)
}

func renderJSON(nodes []domain.Node) ([]byte, error) {
	result := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, map[string]any{"id": node.ID, "name": node.Name, "protocol": node.Protocol,
			"server": node.Address, "port": node.Port, "endpoint_ip": node.EndpointIP, "region": node.Region})
	}
	return json.Marshal(result)
}

func renderOriginal(nodes []domain.Node) ([]byte, error) {
	lines := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if uri := nodeShareURI(node); uri != "" {
			lines = append(lines, uri)
		}
	}
	if len(lines) == 0 {
		return nil, ErrNoCompatibleNodes
	}
	return []byte(strings.Join(lines, "\n")), nil
}

func renderClash(nodes []domain.Node, options RenderOptions) ([]byte, error) {
	proxies := make([]map[string]any, 0, len(nodes))
	names := make(map[string]bool)
	proxyNames := make([]string, 0, len(nodes))
	proxyRegions := make(map[string][]string)
	for _, node := range nodes {
		proxy, err := clashProxy(node, names)
		if err != nil {
			continue
		}
		name := fmt.Sprint(proxy["name"])
		proxies = append(proxies, proxy)
		proxyNames = append(proxyNames, name)
		if node.Region != "" {
			proxyRegions[node.Region] = append(proxyRegions[node.Region], name)
		}
	}
	if len(proxies) == 0 {
		return nil, ErrNoCompatibleNodes
	}
	testURL := options.TestURL
	if testURL == "" {
		testURL = "https://www.gstatic.com/generate_204"
	}
	interval := options.TestInterval
	if interval < 1 {
		interval = 300
	}
	groups := []map[string]any{
		{"name": "节点选择", "type": "select", "proxies": append(append([]string{"AUTO"}, proxyNames...), "DIRECT")},
		{"name": "AUTO", "type": "url-test", "proxies": proxyNames, "url": testURL, "interval": interval},
	}
	regions := make([]string, 0, len(proxyRegions))
	for region := range proxyRegions {
		regions = append(regions, region)
	}
	sort.Strings(regions)
	for _, region := range regions {
		regionNames := proxyRegions[region]
		groups = append(groups, map[string]any{
			"name": region + "节点", "type": "url-test", "proxies": regionNames,
			"url": testURL, "interval": interval,
		})
	}
	doc := map[string]any{
		"mode":         "rule",
		"proxies":      proxies,
		"proxy-groups": groups,
		"rules":        []string{"MATCH,节点选择"},
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("render_clash: %w", err)
	}
	return out, nil
}

func clashProxy(node domain.Node, names map[string]bool) (map[string]any, error) {
	if node.Protocol == "" || node.Address == "" || node.Port < 1 {
		return nil, errors.New("missing type, server or port")
	}
	raw := cloneMap(node.RawConfig)
	delete(raw, "uri")
	raw["name"] = uniqueName(node.Name, names)
	if raw["server"] == nil {
		raw["server"] = node.Address
	}
	if raw["port"] == nil {
		raw["port"] = node.Port
	}
	if raw["type"] == nil {
		raw["type"] = node.Protocol
	}
	return raw, nil
}

func nodeShareURI(node domain.Node) string {
	if value, ok := node.RawConfig["uri"].(string); ok && strings.Contains(value, "://") {
		return normalizeNodeURI(value)
	}
	if node.Address == "" || node.Port < 1 {
		return ""
	}
	if node.Protocol == "vmess" {
		return vmessURI(node)
	}
	if node.Protocol == "ssr" {
		return ssrURI(node)
	}
	u := &url.URL{Scheme: node.Protocol, Host: net.JoinHostPort(node.Address, strconv.Itoa(node.Port)), Fragment: node.Name}
	raw := node.RawConfig
	switch node.Protocol {
	case "trojan":
		if password := rawString(raw, "password"); password != "" {
			u.User = url.User(password)
		} else {
			return ""
		}
	case "vless":
		if uuid := rawString(raw, "uuid"); uuid != "" {
			u.User = url.User(uuid)
		} else {
			return ""
		}
	case "ss", "shadowsocks":
		method, password := rawString(raw, "cipher"), rawString(raw, "password")
		if method == "" || password == "" {
			return ""
		}
		u.User = url.User(method + ":" + password)
	case "socks", "socks5", "http":
		if username := rawString(raw, "username", "user"); username != "" {
			if password := rawString(raw, "password"); password != "" {
				u.User = url.UserPassword(username, password)
			} else {
				u.User = url.User(username)
			}
		}
	case "hysteria2":
		if password := rawString(raw, "password"); password != "" {
			u.User = url.User(password)
		}
	case "tuic":
		uuid, password := rawString(raw, "uuid"), rawString(raw, "password")
		if uuid == "" || password == "" {
			return ""
		}
		u.User = url.UserPassword(uuid, password)
	default:
		return ""
	}
	query := u.Query()
	addShareQueries(query, node)
	u.RawQuery = query.Encode()
	return u.String()
}

func addShareQueries(query url.Values, node domain.Node) {
	raw := node.RawConfig
	setQuery := func(key string, keys ...string) {
		if value := rawString(raw, keys...); value != "" {
			query.Set(key, value)
		}
	}
	setBoolQuery := func(key string, keys ...string) {
		if rawBool(raw, keys...) {
			query.Set(key, "1")
		}
	}

	switch node.Protocol {
	case "vless", "trojan":
		if rawBool(raw, "tls") && rawString(raw, "security") == "" {
			query.Set("security", "tls")
		}
		setQuery("security", "security")
		setQuery("type", "network", "net")
		setQuery("sni", "sni", "servername", "server_name")
		setQuery("flow", "flow")
		setQuery("fp", "fp", "fingerprint")
		setQuery("pbk", "pbk", "public-key")
		setQuery("sid", "sid", "short-id")
		setQuery("spx", "spx", "spiderX", "spider-x")
		setQuery("alpn", "alpn")
		setBoolQuery("insecure", "insecure", "allow_insecure", "skip-cert-verify")
		ws := rawMap(raw, "ws-opts", "ws_opts")
		setNestedQuery(query, "path", raw, ws, "path")
		setNestedQuery(query, "host", raw, ws, "host")
		grpc := rawMap(raw, "grpc-opts", "grpc_opts")
		setNestedQuery(query, "serviceName", raw, grpc, "service-name", "serviceName")
		setNestedQuery(query, "mode", raw, grpc, "mode")
	case "hysteria2":
		setQuery("sni", "sni", "servername", "server_name")
		setBoolQuery("insecure", "insecure", "skip-cert-verify")
		setQuery("obfs", "obfs")
		setQuery("obfs-password", "obfs-password", "obfs_password")
		setQuery("pinSHA256", "pinSHA256", "pin-sha256")
		setQuery("alpn", "alpn")
	case "tuic":
		setQuery("congestion_control", "congestion_control", "congestion-control")
		setQuery("udp_relay_mode", "udp_relay_mode", "udp-relay-mode")
		setQuery("heartbeat", "heartbeat")
		setQuery("sni", "sni", "servername", "server_name")
		setQuery("alpn", "alpn")
		setBoolQuery("allow_insecure", "allow_insecure", "insecure", "skip-cert-verify")
	case "ss", "shadowsocks":
		setQuery("plugin", "plugin")
	case "http", "socks", "socks5":
		setBoolQuery("tls", "tls")
	}
}

func setNestedQuery(query url.Values, key string, primary, nested map[string]any, keys ...string) {
	if value := rawString(primary, keys...); value != "" {
		query.Set(key, value)
		return
	}
	if value := rawString(nested, keys...); value != "" {
		query.Set(key, value)
	}
}

func vmessURI(node domain.Node) string {
	raw := node.RawConfig
	config := map[string]any{
		"v":    "2",
		"ps":   node.Name,
		"add":  node.Address,
		"port": node.Port,
		"id":   rawString(raw, "uuid", "id"),
		"aid":  rawIntOrDefault(raw, 0, "alterId", "alter_id", "aid"),
		"scy":  rawStringOrDefault(raw, "auto", "cipher", "scy"),
		"net":  rawStringOrDefault(raw, "tcp", "network", "net"),
		"type": rawStringOrDefault(raw, "none", "headerType", "header_type"),
	}
	if config["id"] == "" {
		return ""
	}
	if rawBool(raw, "tls") || rawString(raw, "tls") == "tls" {
		config["tls"] = "tls"
	}
	if value := rawString(raw, "sni", "servername", "server_name"); value != "" {
		config["sni"] = value
	}
	if value := rawString(raw, "alpn"); value != "" {
		config["alpn"] = value
	}
	ws := rawMap(raw, "ws-opts", "ws_opts")
	if value := rawString(raw, "path"); value != "" {
		config["path"] = value
	} else if value := rawString(ws, "path"); value != "" {
		config["path"] = value
	}
	if value := rawString(raw, "host"); value != "" {
		config["host"] = value
	} else if value := rawString(ws, "host"); value != "" {
		config["host"] = value
	} else if headers := rawMap(ws, "headers"); headers != nil {
		if value := rawString(headers, "Host", "host"); value != "" {
			config["host"] = value
		}
	}
	grpc := rawMap(raw, "grpc-opts", "grpc_opts")
	if value := rawString(raw, "serviceName", "service-name"); value != "" {
		config["path"] = value
	} else if value := rawString(grpc, "grpc-service-name", "serviceName", "service-name"); value != "" {
		config["path"] = value
	}
	data, err := json.Marshal(config)
	if err != nil {
		return ""
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(data)
}

func ssrURI(node domain.Node) string {
	raw := node.RawConfig
	protocol, method, obfs := rawString(raw, "protocol"), rawString(raw, "method", "cipher"), rawString(raw, "obfs")
	password := rawString(raw, "password")
	if protocol == "" || method == "" || obfs == "" || password == "" {
		return ""
	}
	fields := []string{node.Address, strconv.Itoa(node.Port), protocol, method, obfs, base64.RawURLEncoding.EncodeToString([]byte(password))}
	params := url.Values{}
	for key, value := range map[string]string{
		"obfsparam":  rawString(raw, "obfs-param", "obfs_param"),
		"protoparam": rawString(raw, "protocol-param", "protocol_param"),
		"remarks":    node.Name,
	} {
		if value != "" {
			params.Set(key, base64.RawURLEncoding.EncodeToString([]byte(value)))
		}
	}
	payload := strings.Join(fields, ":")
	if encoded := params.Encode(); encoded != "" {
		payload += "/?" + encoded
	}
	return "ssr://" + base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func rawString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok || value == nil {
			continue
		}
		if text, ok := value.(string); ok {
			if strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func rawStringOrDefault(raw map[string]any, fallback string, keys ...string) string {
	if value := rawString(raw, keys...); value != "" {
		return value
	}
	return fallback
}

func rawInt(raw map[string]any, keys ...string) int {
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			if number, err := intValue(value); err == nil {
				return number
			}
		}
	}
	return -1
}

func rawIntOrDefault(raw map[string]any, fallback int, keys ...string) int {
	if value := rawInt(raw, keys...); value >= 0 {
		return value
	}
	return fallback
}

func rawBool(raw map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			return typed == "1" || strings.EqualFold(typed, "true") || strings.EqualFold(typed, "yes")
		case float64:
			return typed != 0
		}
	}
	return false
}

func rawMap(raw map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case map[string]any:
			return typed
		case map[any]any:
			out := make(map[string]any, len(typed))
			for nestedKey, nestedValue := range typed {
				out[fmt.Sprint(nestedKey)] = nestedValue
			}
			return out
		}
	}
	return nil
}

func renderBase64(nodes []domain.Node) ([]byte, error) {
	lines := make([]string, 0, len(nodes))
	for _, node := range nodes {
		uri := nodeShareURI(node)
		if !strings.Contains(uri, "://") {
			continue
		}

		lines = append(lines, uri)
	}
	if len(lines) == 0 {
		return nil, ErrNoCompatibleNodes
	}
	return []byte(base64.StdEncoding.EncodeToString([]byte(strings.Join(lines, "\n")))), nil
}

func renderSingBox(nodes []domain.Node) ([]byte, error) {
	outbounds := make([]map[string]any, 0, len(nodes))
	names := make([]string, 0, len(nodes))
	used := make(map[string]bool)
	regionOutbounds := make(map[string][]string)
	for _, node := range nodes {
		outbound, err := singBoxOutbound(node)
		if err != nil {
			continue
		}
		tag := uniqueName(node.Name, used)
		outbound["tag"] = tag
		outbounds = append(outbounds, outbound)
		names = append(names, tag)
		if node.Region != "" {
			regionOutbounds[node.Region] = append(regionOutbounds[node.Region], tag)
		}
	}

	if len(outbounds) == 0 {
		return nil, ErrNoCompatibleNodes
	}
	groups := []map[string]any{
		{"type": "selector", "tag": "节点选择", "outbounds": append(append([]string{"AUTO"}, names...), "direct")},
		{"type": "urltest", "tag": "AUTO", "outbounds": names, "url": "https://www.gstatic.com/generate_204"},
	}
	regions := make([]string, 0, len(regionOutbounds))
	for region := range regionOutbounds {
		regions = append(regions, region)
	}
	sort.Strings(regions)
	for _, region := range regions {
		groups = append(groups, map[string]any{
			"type": "urltest", "tag": region + "节点", "outbounds": regionOutbounds[region],
			"url": "https://www.gstatic.com/generate_204",
		})
	}
	outbounds = append(groups, outbounds...)
	outbounds = append(outbounds, map[string]any{"type": "direct", "tag": "direct"})
	return json.Marshal(map[string]any{"outbounds": outbounds, "route": map[string]any{"final": "节点选择"}})
}

func RenderSubscription(nodes []domain.Node, format Format) ([]byte, error) {
	return Render(nodes, RenderOptions{Format: format})
}

func singBoxOutbound(node domain.Node) (map[string]any, error) {
	raw := node.RawConfig
	switch node.Protocol {
	case "socks5", "socks":
		out := map[string]any{"type": "socks", "tag": node.Name, "server": node.Address, "server_port": node.Port}
		addSingBoxAuth(out, raw)
		return out, nil
	case "http":
		out := map[string]any{"type": "http", "tag": node.Name, "server": node.Address, "server_port": node.Port}
		addSingBoxAuth(out, raw)
		return out, nil
	case "trojan", "vless", "vmess", "shadowsocks", "ss":
		protocol := map[string]string{"ss": "shadowsocks", "shadowsocks": "shadowsocks"}[node.Protocol]
		if protocol == "" {
			protocol = node.Protocol
		}
		out := map[string]any{"type": protocol, "tag": node.Name, "server": node.Address, "server_port": node.Port}
		switch node.Protocol {
		case "trojan":
			if password := rawString(raw, "password"); password != "" {
				out["password"] = password
			} else {
				return nil, errors.New("missing password")
			}
		case "vless", "vmess":
			if uuid := rawString(raw, "uuid", "id"); uuid != "" {
				out["uuid"] = uuid
			} else {
				return nil, errors.New("missing uuid")
			}
			if alterID := rawInt(raw, "alterId", "alter_id", "aid"); alterID >= 0 && node.Protocol == "vmess" {
				out["alter_id"] = alterID
			}
			if security := rawString(raw, "cipher", "scy"); security != "" && node.Protocol == "vmess" {
				out["security"] = security
			}
			if flow := rawString(raw, "flow"); flow != "" {
				out["flow"] = flow
			}
		case "ss", "shadowsocks":
			method, password := rawString(raw, "cipher", "method"), rawString(raw, "password")
			if method == "" || password == "" {
				return nil, errors.New("missing method or password")
			}
			out["method"], out["password"] = method, password
		}
		addSingBoxTLS(out, raw)
		addSingBoxTransport(out, raw)
		return out, nil
	default:
		return nil, errors.New("unsupported protocol")
	}
}

func addSingBoxAuth(out map[string]any, raw map[string]any) {
	if username := rawString(raw, "username", "user"); username != "" {
		out["username"] = username
	}
	if password := rawString(raw, "password"); password != "" {
		out["password"] = password
	}
}

func addSingBoxTLS(out map[string]any, raw map[string]any) {
	security := rawString(raw, "security")
	if !rawBool(raw, "tls") && security == "" && rawString(raw, "sni", "servername", "server_name") == "" {
		return
	}
	tls := map[string]any{"enabled": rawBool(raw, "tls") || security == "tls" || security == "reality"}
	if serverName := rawString(raw, "sni", "servername", "server_name"); serverName != "" {
		tls["server_name"] = serverName
	}
	if rawBool(raw, "insecure", "allow_insecure", "skip-cert-verify") {
		tls["insecure"] = true
	}
	if security == "reality" {
		tls["reality"] = true
		if publicKey := rawString(raw, "pbk", "public-key"); publicKey != "" {
			tls["reality"] = map[string]any{"enabled": true, "public_key": publicKey, "short_id": rawString(raw, "sid", "short-id")}
		}
	}
	out["tls"] = tls
}

func addSingBoxTransport(out map[string]any, raw map[string]any) {
	network := strings.ToLower(rawString(raw, "network", "net"))
	if network == "" || network == "tcp" {
		return
	}
	transport := map[string]any{"type": network}
	switch network {
	case "ws", "websocket":
		transport["type"] = "ws"
		ws := rawMap(raw, "ws-opts", "ws_opts")
		if path := rawString(raw, "path"); path != "" {
			transport["path"] = path
		} else if path := rawString(ws, "path"); path != "" {
			transport["path"] = path
		}
		if headers := rawMap(ws, "headers"); headers != nil {
			transport["headers"] = headers
		} else if host := rawString(raw, "host"); host != "" {
			transport["headers"] = map[string]any{"Host": host}
		}
	case "grpc":
		grpc := rawMap(raw, "grpc-opts", "grpc_opts")
		if serviceName := rawString(raw, "serviceName", "service-name"); serviceName != "" {
			transport["service_name"] = serviceName
		} else if serviceName := rawString(grpc, "serviceName", "service-name", "grpc-service-name"); serviceName != "" {
			transport["service_name"] = serviceName
		}
	default:
		return
	}
	out["transport"] = transport
}

func uniqueName(name string, used map[string]bool) string {
	if name == "" {
		name = "proxy"
	}
	base := name
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	used[name] = true
	return name
}
