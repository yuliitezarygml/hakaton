package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// DockerHandler управляет Docker-контейнерами через Unix socket
type DockerHandler struct {
	adminHandler *AdminHandler
}

func NewDockerHandler(admin *AdminHandler) *DockerHandler {
	return &DockerHandler{adminHandler: admin}
}

// dockerAPICall — делает запрос к Docker Engine API через Unix socket
func dockerAPICall(method, path, body string) ([]byte, int, error) {
	conn, err := net.Dial("unix", "/var/run/docker.sock")
	if err != nil {
		return nil, 0, fmt.Errorf("docker socket недоступен: %w", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(15 * time.Second))

	var reqBody string
	contentLen := 0
	if body != "" {
		reqBody = body
		contentLen = len(body)
	}

	req := fmt.Sprintf("%s %s HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		method, path, contentLen, reqBody)

	if _, err := conn.Write([]byte(req)); err != nil {
		return nil, 0, err
	}

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// Читаем статус-строку
	scanner.Scan()
	statusLine := scanner.Text()
	statusCode := 200
	fmt.Sscanf(statusLine, "HTTP/1.1 %d", &statusCode)

	// Пропускаем заголовки
	for scanner.Scan() {
		if scanner.Text() == "" {
			break
		}
	}

	// Читаем тело
	var sb strings.Builder
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
		sb.WriteString("\n")
	}

	return []byte(sb.String()), statusCode, nil
}

// KnownServices — список сервисов из docker-compose
var KnownServices = []string{
	"backend", "telegram-bot", "postgres", "redis", "fact-guard", "nginx",
}

type ContainerInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Image  string `json:"image"`
	Status string `json:"status"`
	State  string `json:"state"`
	Ports  string `json:"ports"`
}

// ListContainers — GET /api/admin/docker/containers
func (h *DockerHandler) ListContainers(w http.ResponseWriter, r *http.Request) {
	data, code, err := dockerAPICall("GET", "/v1.44/containers/json?all=true", "")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if code != 200 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		w.Write(data)
		return
	}

	// Парсим массив контейнеров
	var raw []map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		// Возможно chunked encoding — просто возвращаем как есть
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
		return
	}

	var result []ContainerInfo
	for _, c := range raw {
		name := ""
		if names, ok := c["Names"].([]interface{}); ok && len(names) > 0 {
			name = strings.TrimPrefix(fmt.Sprintf("%v", names[0]), "/")
		}
		image := fmt.Sprintf("%v", c["Image"])
		status := fmt.Sprintf("%v", c["Status"])
		state := fmt.Sprintf("%v", c["State"])
		id := fmt.Sprintf("%v", c["Id"])
		if len(id) > 12 {
			id = id[:12]
		}

		// Составляем порты
		ports := ""
		if portsRaw, ok := c["Ports"].([]interface{}); ok {
			var parts []string
			for _, p := range portsRaw {
				if pm, ok := p.(map[string]interface{}); ok {
					pub := fmt.Sprintf("%v", pm["PublicPort"])
					priv := fmt.Sprintf("%v", pm["PrivatePort"])
					if pub != "<nil>" && pub != "0" {
						parts = append(parts, pub+":"+priv)
					}
				}
			}
			ports = strings.Join(parts, ", ")
		}

		result = append(result, ContainerInfo{
			ID:     id,
			Name:   name,
			Image:  image,
			Status: status,
			State:  state,
			Ports:  ports,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

type dockerActionRequest struct {
	ContainerName string `json:"name"`
	Action        string `json:"action"` // start | stop | restart
}

// ContainerAction — POST /api/admin/docker/action
func (h *DockerHandler) ContainerAction(w http.ResponseWriter, r *http.Request) {
	var req dockerActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	action := req.Action
	if action != "start" && action != "stop" && action != "restart" {
		http.Error(w, "invalid action", http.StatusBadRequest)
		return
	}

	// Сначала найдём ID контейнера по имени
	data, _, err := dockerAPICall("GET", "/v1.44/containers/json?all=true", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	var containers []map[string]interface{}
	json.Unmarshal(data, &containers)

	containerID := ""
	for _, c := range containers {
		if names, ok := c["Names"].([]interface{}); ok {
			for _, n := range names {
				name := strings.TrimPrefix(fmt.Sprintf("%v", n), "/")
				if name == req.ContainerName || strings.Contains(name, req.ContainerName) {
					containerID = fmt.Sprintf("%v", c["Id"])
					break
				}
			}
		}
	}

	if containerID == "" {
		http.Error(w, "container not found: "+req.ContainerName, http.StatusNotFound)
		return
	}

	path := fmt.Sprintf("/v1.44/containers/%s/%s", containerID, action)
	_, code, err := dockerAPICall("POST", path, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[DOCKER] %s → %s (code: %d)", action, req.ContainerName, code)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":     true,
		"action": action,
		"name":   req.ContainerName,
		"code":   code,
	})
}

// StreamContainerLogs — WebSocket /api/admin/docker/logs
func (h *DockerHandler) StreamContainerLogs(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token != h.adminHandler.cfg.AdminToken {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	containerName := r.URL.Query().Get("container")
	if containerName == "" {
		http.Error(w, "container param required", http.StatusBadRequest)
		return
	}

	// Найдём ID по имени
	data, _, err := dockerAPICall("GET", "/v1.44/containers/json?all=true", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	var containers []map[string]interface{}
	json.Unmarshal(data, &containers)

	containerID := ""
	for _, c := range containers {
		if names, ok := c["Names"].([]interface{}); ok {
			for _, n := range names {
				name := strings.TrimPrefix(fmt.Sprintf("%v", n), "/")
				if name == containerName || strings.Contains(name, containerName) {
					containerID = fmt.Sprintf("%v", c["Id"])
					break
				}
			}
		}
	}

	if containerID == "" {
		http.Error(w, "container not found", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[DOCKER] WS upgrade error: %v", err)
		return
	}
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				close(done)
				return
			}
		}
	}()

	// Подключаемся к Docker logs API (raw TCP)
	sockConn, err := net.Dial("unix", "/var/run/docker.sock")
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("[ERROR] Docker socket: "+err.Error()))
		return
	}
	defer sockConn.Close()

	// Запрашиваем логи с follow + последние 100 строк
	logPath := fmt.Sprintf("/v1.41/containers/%s/logs?stdout=true&stderr=true&follow=true&tail=100×tamps=true", containerID)
	req2 := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n", logPath)
	sockConn.Write([]byte(req2))

	reader := bufio.NewReader(sockConn)

	// Пропускаем HTTP заголовки ответа
	for {
		line, err := reader.ReadString('\n')
		if err != nil || strings.TrimSpace(line) == "" {
			break
		}
	}

	// Стримим логи по строкам
	buf := make([]byte, 8)
	for {
		select {
		case <-done:
			return
		default:
		}

		// Docker multiplex stream: 8-byte header (stream type + size)
		_, err := reader.Read(buf[:8])
		if err != nil {
			break
		}
		size := int(buf[4])<<24 | int(buf[5])<<16 | int(buf[6])<<8 | int(buf[7])
		if size <= 0 {
			continue
		}
		logBuf := make([]byte, size)
		reader.Read(logBuf)

		line := strings.TrimRight(string(logBuf), "\r\n")
		if line == "" {
			continue
		}

		if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
			return
		}
	}
}
