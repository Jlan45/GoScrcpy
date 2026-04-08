package main

import (
	"context"
	"embed"
	"encoding/binary"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/Jlan45/GoScrcpy/adb"
	"github.com/Jlan45/GoScrcpy/client"
	"github.com/Jlan45/GoScrcpy/protocol"
)

//go:embed index.html
var staticFS embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// deviceSession holds a scrcpy connection to one device and its WS subscribers.
type deviceSession struct {
	serial string
	client *client.Client
	cancel context.CancelFunc

	mu           sync.RWMutex
	wsClients    map[*wsClient]struct{}
	lastConfig   []byte
	lastKeyFrame []byte
}

func newDeviceSession(serial string, opts client.Options) (*deviceSession, error) {
	opts.Serial = serial
	ctx, cancel := context.WithCancel(context.Background())
	c := client.NewClient(opts)
	if err := c.Start(ctx); err != nil {
		cancel()
		return nil, err
	}
	ds := &deviceSession{
		serial:    serial,
		client:    c,
		cancel:    cancel,
		wsClients: make(map[*wsClient]struct{}),
	}
	go ds.broadcastVideo()
	return ds, nil
}

func (ds *deviceSession) stop() {
	ds.cancel()
	ds.client.Stop()
	ds.mu.Lock()
	for wc := range ds.wsClients {
		wc.conn.Close()
	}
	ds.wsClients = nil
	ds.mu.Unlock()
}

func (ds *deviceSession) addWS(wc *wsClient) {
	ds.mu.Lock()
	ds.wsClients[wc] = struct{}{}
	ds.mu.Unlock()
}

func (ds *deviceSession) removeWS(wc *wsClient) {
	ds.mu.Lock()
	delete(ds.wsClients, wc)
	ds.mu.Unlock()
}

func (ds *deviceSession) broadcast(data []byte) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	for wc := range ds.wsClients {
		if err := wc.send(data); err != nil {
			go wc.conn.Close()
		}
	}
}

func (ds *deviceSession) broadcastVideo() {
	vStream := ds.client.VideoStream()
	if vStream == nil {
		return
	}
	for pkt := range vStream {
		buf := make([]byte, 10+len(pkt.Data))
		buf[0] = 0
		var flags byte
		if pkt.IsKeyFrame {
			flags |= 0x01
		}
		if pkt.IsConfig {
			flags |= 0x02
		}
		buf[1] = flags
		binary.BigEndian.PutUint64(buf[2:10], uint64(pkt.PTS))
		copy(buf[10:], pkt.Data)

		ds.mu.Lock()
		if pkt.IsConfig {
			ds.lastConfig = append(ds.lastConfig[:0], buf...)
		}
		if pkt.IsKeyFrame {
			ds.lastKeyFrame = append(ds.lastKeyFrame[:0], buf...)
		}
		ds.mu.Unlock()

		ds.broadcast(buf)
	}
	log.Printf("[%s] video stream ended", ds.serial)
}

func (ds *deviceSession) info() map[string]any {
	vw, vh := ds.client.VideoSize()
	return map[string]any{
		"serial": ds.serial,
		"device": ds.client.DeviceName(),
		"width":  vw,
		"height": vh,
		"codec":  protocol.CodecName(ds.client.VideoCodecID()),
	}
}

// --- wsClient ---

type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (wc *wsClient) send(data []byte) error {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	return wc.conn.WriteMessage(websocket.BinaryMessage, data)
}

// --- server ---

type server struct {
	baseOpts client.Options

	mu       sync.RWMutex
	sessions map[string]*deviceSession // serial -> session
}

func (s *server) getSession(serial string) *deviceSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[serial]
}

func (s *server) connectDevice(serial string) (*deviceSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ds, ok := s.sessions[serial]; ok {
		return ds, nil
	}
	ds, err := newDeviceSession(serial, s.baseOpts)
	if err != nil {
		return nil, err
	}
	s.sessions[serial] = ds
	log.Printf("[%s] connected: %s", serial, ds.client.DeviceName())
	return ds, nil
}

func (s *server) disconnectDevice(serial string) {
	s.mu.Lock()
	ds, ok := s.sessions[serial]
	if ok {
		delete(s.sessions, serial)
	}
	s.mu.Unlock()
	if ok {
		ds.stop()
		log.Printf("[%s] disconnected", serial)
	}
}

func (s *server) listSessions() []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []map[string]any
	for _, ds := range s.sessions {
		list = append(list, ds.info())
	}
	return list
}

// --- HTTP handlers ---

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, _ := staticFS.ReadFile("index.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// GET /api/devices — list ADB devices + connected sessions
func (s *server) handleAPIDevices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	adbDevices, err := adb.ListDevices()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	s.mu.RLock()
	connectedSet := make(map[string]bool)
	for serial := range s.sessions {
		connectedSet[serial] = true
	}
	s.mu.RUnlock()

	type deviceInfo struct {
		Serial    string `json:"serial"`
		State     string `json:"state"`
		Connected bool   `json:"connected"`
		Name      string `json:"name,omitempty"`
	}
	var result []deviceInfo
	for _, d := range adbDevices {
		di := deviceInfo{Serial: d.Serial, State: d.State, Connected: connectedSet[d.Serial]}
		if ds := s.getSession(d.Serial); ds != nil {
			di.Name = ds.client.DeviceName()
		}
		result = append(result, di)
	}
	json.NewEncoder(w).Encode(result)
}

// POST /api/connect?serial=xxx
func (s *server) handleAPIConnect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	serial := r.URL.Query().Get("serial")
	if serial == "" {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "serial required"})
		return
	}
	ds, err := s.connectDevice(serial)
	if err != nil {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(ds.info())
}

// POST /api/disconnect?serial=xxx
func (s *server) handleAPIDisconnect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	serial := r.URL.Query().Get("serial")
	if serial == "" {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "serial required"})
		return
	}
	s.disconnectDevice(serial)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// GET /api/sessions — list active scrcpy sessions
func (s *server) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.listSessions())
}

// WS /ws?serial=xxx
func (s *server) handleWS(w http.ResponseWriter, r *http.Request) {
	serial := r.URL.Query().Get("serial")
	if serial == "" {
		http.Error(w, "serial required", 400)
		return
	}

	ds := s.getSession(serial)
	if ds == nil {
		http.Error(w, "device not connected, call /api/connect first", 404)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}

	wc := &wsClient{conn: conn}
	ds.addWS(wc)
	defer func() {
		ds.removeWS(wc)
		conn.Close()
	}()

	// Send device info
	info, _ := json.Marshal(ds.info())
	wc.send(append([]byte{2}, info...))

	// Send cached config + keyframe
	ds.mu.RLock()
	cfg := ds.lastConfig
	kf := ds.lastKeyFrame
	ds.mu.RUnlock()
	if cfg != nil {
		wc.send(cfg)
	}
	if kf != nil {
		wc.send(kf)
	}

	// Read control messages
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if len(data) == 0 {
			continue
		}
		handleControl(ds.client, data)
	}
}

func handleControl(c *client.Client, data []byte) {
	switch data[0] {
	case 0: // touch
		if len(data) < 14 {
			return
		}
		action := data[1]
		x := int32(binary.BigEndian.Uint32(data[2:6]))
		y := int32(binary.BigEndian.Uint32(data[6:10]))
		sw := binary.BigEndian.Uint16(data[10:12])
		sh := binary.BigEndian.Uint16(data[12:14])
		pressure := float32(1.0)
		if action == protocol.ActionUp {
			pressure = 0
		}
		c.SendControl(&protocol.InjectTouchEvent{
			Action:    action,
			PointerID: protocol.PointerIDGenericFinger,
			Position:  protocol.Position{X: x, Y: y, ScreenWidth: sw, ScreenHeight: sh},
			Pressure:  pressure,
		})
	case 1: // key
		if len(data) < 6 {
			return
		}
		c.SendControl(&protocol.InjectKeycode{Action: data[1], Keycode: binary.BigEndian.Uint32(data[2:6])})
	case 2: // scroll
		if len(data) < 17 {
			return
		}
		x := int32(binary.BigEndian.Uint32(data[1:5]))
		y := int32(binary.BigEndian.Uint32(data[5:9]))
		sw := binary.BigEndian.Uint16(data[9:11])
		sh := binary.BigEndian.Uint16(data[11:13])
		hs := int16(binary.BigEndian.Uint16(data[13:15]))
		vs := int16(binary.BigEndian.Uint16(data[15:17]))
		c.SendControl(&protocol.InjectScrollEvent{
			Position: protocol.Position{X: x, Y: y, ScreenWidth: sw, ScreenHeight: sh},
			HScroll:  float32(hs), VScroll: float32(vs),
		})
	case 3:
		sendKeyPair(c, 4)
	case 4:
		sendKeyPair(c, 3)
	case 5:
		sendKeyPair(c, 187)
	case 6:
		sendKeyPair(c, 26)
	}
}

func sendKeyPair(c *client.Client, keycode uint32) {
	c.SendControl(&protocol.InjectKeycode{Action: protocol.ActionDown, Keycode: keycode})
	c.SendControl(&protocol.InjectKeycode{Action: protocol.ActionUp, Keycode: keycode})
}

// --- main ---

func main() {
	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	baseOpts := client.DefaultOptions()
	baseOpts.ServerPath = os.Getenv("SCRCPY_SERVER_PATH")
	baseOpts.ServerVersion = "3.3.4"

	srv := &server{
		baseOpts: baseOpts,
		sessions: make(map[string]*deviceSession),
	}

	// Auto-connect if DEVICE_SERIAL is set
	if serial := os.Getenv("DEVICE_SERIAL"); serial != "" {
		if _, err := srv.connectDevice(serial); err != nil {
			log.Printf("auto-connect %s failed: %v", serial, err)
		}
	}

	http.HandleFunc("/", srv.handleIndex)
	http.HandleFunc("/api/devices", srv.handleAPIDevices)
	http.HandleFunc("/api/connect", srv.handleAPIConnect)
	http.HandleFunc("/api/disconnect", srv.handleAPIDisconnect)
	http.HandleFunc("/api/sessions", srv.handleAPISessions)
	http.HandleFunc("/ws", srv.handleWS)

	log.Printf("Listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
