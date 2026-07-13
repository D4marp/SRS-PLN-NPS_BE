package health

import (
	"context"
	"hash/crc32"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const onlineWindow = 90 * time.Second

type ClientStatus struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Name          string `json:"name"`
	RoomName      string `json:"roomName"`
	IP            string `json:"ip"`
	UserAgent     string `json:"userAgent"`
	Origin        string `json:"origin"`
	LastMethod    string `json:"lastMethod"`
	LastPath      string `json:"lastPath"`
	LastStatus    int    `json:"lastStatus"`
	LastSeen      int64  `json:"lastSeen"`
	AgeSeconds    int64  `json:"ageSeconds"`
	Online        bool   `json:"online"`
	TotalRequests int64  `json:"totalRequests"`
}

type PingRequest struct {
	ClientID   string `json:"clientId"`
	ClientType string `json:"clientType"`
	ClientName string `json:"clientName"`
}

type Monitor struct {
	mu      sync.Mutex
	clients map[string]*ClientStatus
}

func NewMonitor() *Monitor {
	return &Monitor{clients: map[string]*ClientStatus{}}
}

func (m *Monitor) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if shouldSkip(c.Request.URL.Path) {
			return
		}
		m.Record(c, PingRequest{})
	}
}

func (m *Monitor) Record(c *gin.Context, req PingRequest) ClientStatus {
	now := time.Now()
	ip := clientIP(c.Request)
	ua := strings.TrimSpace(c.GetHeader("User-Agent"))
	origin := strings.TrimSpace(c.GetHeader("Origin"))
	clientType := normalizeType(firstNonEmpty(req.ClientType, c.GetHeader("X-Client-Type"), c.Query("clientType"), detectClientType(c)))
	clientID := firstNonEmpty(req.ClientID, c.GetHeader("X-Client-Id"), c.Query("clientId"))
	if clientID == "" {
		clientID = inferredClientID(clientType, ip, ua)
	}
	clientName := firstNonEmpty(req.ClientName, c.GetHeader("X-Client-Name"), c.Query("clientName"), defaultName(clientType, ip))
	roomName := ""
	if clientType == "mobile" {
		roomName = roomNameByIP(ip)
	}
	if roomName != "" && clientName == "Mobile App" {
		clientName = roomName
	}
	key := clientType + ":" + clientID

	m.mu.Lock()
	defer m.mu.Unlock()

	status := m.clients[key]
	if status == nil {
		status = &ClientStatus{
			ID:        clientID,
			Type:      clientType,
			Name:      clientName,
			RoomName:  roomName,
			IP:        ip,
			UserAgent: ua,
			Origin:    origin,
		}
		m.clients[key] = status
	}

	status.Name = clientName
	status.RoomName = roomName
	status.IP = ip
	status.UserAgent = ua
	status.Origin = origin
	status.LastMethod = c.Request.Method
	status.LastPath = c.Request.URL.Path
	status.LastStatus = c.Writer.Status()
	status.LastSeen = now.UnixMilli()
	status.AgeSeconds = 0
	status.Online = true
	status.TotalRequests++

	return *status
}

func (m *Monitor) Snapshot() gin.H {
	now := time.Now()
	probes := probeRoomDevices()
	clients := m.clientsSnapshot(now, probes)
	summary := map[string]gin.H{}
	for _, client := range clients {
		item, ok := summary[client.Type]
		if !ok {
			item = gin.H{"total": 0, "online": 0, "offline": 0}
			summary[client.Type] = item
		}
		item["total"] = item["total"].(int) + 1
		if client.Online {
			item["online"] = item["online"].(int) + 1
		} else {
			item["offline"] = item["offline"].(int) + 1
		}
	}
	return gin.H{
		"status":       "ok",
		"onlineWindow": int64(onlineWindow.Seconds()),
		"generatedAt":  now.UnixMilli(),
		"summary":      summary,
		"clients":      clients,
	}
}

func (m *Monitor) Summary() gin.H {
	snapshot := m.Snapshot()
	return gin.H{
		"onlineWindow": snapshot["onlineWindow"],
		"summary":      snapshot["summary"],
	}
}

func (m *Monitor) clientsSnapshot(now time.Time, probes map[string]bool) []ClientStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	clients := make([]ClientStatus, 0, len(m.clients)+len(roomDevices()))
	seenRooms := map[string]bool{}
	for _, item := range m.clients {
		copy := *item
		lastSeen := time.UnixMilli(copy.LastSeen)
		copy.AgeSeconds = int64(now.Sub(lastSeen).Seconds())
		copy.Online = now.Sub(lastSeen) <= onlineWindow
		if copy.Type == "mobile" && copy.RoomName != "" {
			if online, ok := probes[copy.IP]; ok {
				copy.Online = online
				seenRooms[copy.IP] = true
			}
		}
		clients = append(clients, copy)
	}
	for _, room := range roomDevices() {
		if seenRooms[room.IP] {
			continue
		}
		clients = append(clients, ClientStatus{
			ID:       room.Name,
			Type:     "mobile",
			Name:     room.Name,
			RoomName: room.Name,
			IP:       room.IP,
			LastPath: "network ping",
			Online:   probes[room.IP],
		})
	}
	sort.Slice(clients, func(i, j int) bool {
		if clients[i].Online != clients[j].Online {
			return clients[i].Online
		}
		if clients[i].RoomName != "" || clients[j].RoomName != "" {
			return clients[i].RoomName < clients[j].RoomName
		}
		return clients[i].LastSeen > clients[j].LastSeen
	})
	return clients
}

func shouldSkip(path string) bool {
	return path == "/health/clients"
}

func detectClientType(c *gin.Context) string {
	ua := strings.ToLower(c.GetHeader("User-Agent"))
	origin := strings.ToLower(c.GetHeader("Origin"))
	referer := strings.ToLower(c.GetHeader("Referer"))
	if strings.Contains(ua, "dart") || strings.Contains(ua, "dio") || strings.Contains(ua, "flutter") || strings.Contains(ua, "okhttp") {
		return "mobile"
	}
	if strings.Contains(origin, ":3000") || strings.Contains(referer, ":3000") || strings.Contains(ua, "mozilla") {
		return "web"
	}
	return "api"
}

func normalizeType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "web", "mobile", "api":
		return value
	default:
		return "api"
	}
}

func defaultName(clientType, ip string) string {
	switch clientType {
	case "web":
		return "Admin Web"
	case "mobile":
		if room := roomNameByIP(ip); room != "" {
			return room
		}
		return "Mobile App"
	default:
		if ip != "" {
			return "API Client " + ip
		}
		return "API Client"
	}
}

func roomNameByIP(ip string) string {
	for _, room := range roomDevices() {
		if room.IP == ip {
			return room.Name
		}
	}
	return ""
}

type roomDevice struct {
	Name string
	IP   string
}

func roomDevices() []roomDevice {
	return []roomDevice{
		{Name: "governor", IP: "10.7.41.44"},
		{Name: "boiler", IP: "10.7.41.37"},
		{Name: "msv", IP: "10.7.41.26"},
		{Name: "tab reseptionis", IP: "10.7.41.59"},
		{Name: "generator", IP: "10.7.41.35"},
		{Name: "generator2", IP: "10.7.41.33"},
		{Name: "turbin", IP: "10.7.41.60"},
		{Name: "hall", IP: "10.7.41.61"},
	}
}

func probeRoomDevices() map[string]bool {
	devices := roomDevices()
	results := make(map[string]bool, len(devices))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, device := range devices {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			online := pingIP(ip)
			mu.Lock()
			results[ip] = online
			mu.Unlock()
		}(device.IP)
	}

	wg.Wait()
	return results
}

func pingIP(ip string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()

	args := []string{"-c", "1", "-W", "1", ip}
	if runtime.GOOS == "windows" {
		args = []string{"-n", "1", "-w", "1000", ip}
	}

	cmd := exec.CommandContext(ctx, "ping", args...)
	return cmd.Run() == nil
}

func inferredClientID(clientType, ip, ua string) string {
	if clientType == "web" {
		return "admin-web"
	}
	if ip == "" {
		ip = "unknown"
	}
	return ip + "-" + hashText(ua)
}

func hashText(value string) string {
	return strings.ToLower(strings.TrimLeft(strings.TrimRight(strings.ToUpper(formatHex(crc32.ChecksumIEEE([]byte(value)))), " "), "0"))
}

func formatHex(value uint32) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		out[i] = hex[value&0xf]
		value >>= 4
	}
	return string(out)
}

func clientIP(r *http.Request) string {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value == "" {
			continue
		}
		parts := strings.Split(value, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
