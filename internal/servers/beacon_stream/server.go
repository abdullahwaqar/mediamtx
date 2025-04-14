package beacon_stream

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/gorilla/websocket"
	"github.com/pion/stun/v3"
	"github.com/pion/webrtc/v4"
)

// Message defines the structure for signaling messages
type Message struct {
	SDP       string `json:"sdp,omitempty"`
	Candidate string `json:"candidate,omitempty"`
}

// GPSData represents the structure of GPS data to be sent
type GPSData struct {
	MarkerID int     `json:"markerId"`
	AngleX   float64 `json:"angle_x"`
	AngleY   float64 `json:"angle_y"`
	Distance float64 `json:"distance"`
}

// Packet is an interface that all packet types implement
type Packet interface {
	GetType() string
}

// CommonPacket is used to determine the type of incoming JSON data
type CommonPacket struct {
	Type string `json:"type"`
}

// AttitudePacket represents a packet of type "attitude"
type AttitudePacket struct {
	Type      string     `json:"type"`
	Values    [3]float64 `json:"values"`
	Timestamp int64      `json:"timestamp"`
}

// GetType returns the packet type for AttitudePacket.
func (a AttitudePacket) GetType() string {
	return a.Type
}

// MarkerPacket represents a packet of type "marker"
type MarkerPacket struct {
	Type     string  `json:"type"`
	MarkerID int     `json:"markerId"`
	AngleX   float64 `json:"angle_x"`
	AngleY   float64 `json:"angle_y"`
	Distance float64 `json:"distance"`
}

// GetType returns the packet type for MarkerPacket
func (m MarkerPacket) GetType() string {
	return m.Type
}

// PacketWrapper is a wrapper that holds any type of Packet
type PacketWrapper struct {
	Packet Packet
}

var (
	upgrader = websocket.Upgrader{
		WriteBufferSize: 1024,
		// Allow all origins for testing
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	dataChannels []*webrtc.DataChannel
	dcMux        sync.Mutex
	once         sync.Once // * Ensures broadcastGPSData starts only once
)

// * Client represents a connected WebSocket client
type Client struct {
	conn *websocket.Conn
	mu   sync.Mutex

	ICEServers *conf.WebRTCICEServers
	conf       *conf.GPSConfig

	rawDataLog bool
}

// StartLocalStunServer runs a minimal STUN server on the given port.
// The STUN server will respond with host-based XOR-MAPPED-ADDRESS.
func StartLocalStunServer(port string) {
	go func() {
		addr, err := net.ResolveUDPAddr("udp", port)
		if err != nil {
			log.Printf("Failed to resolve STUN listen addr: %v", err)
			return
		}
		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			log.Printf("Failed to listen on STUN UDP addr: %v", err)
			return
		}
		defer conn.Close()

		buf := make([]byte, 1500)
		log.Printf("Local STUN server listening on %s", addr)

		for {
			n, rAddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				log.Println("STUN read error:", err)
				continue
			}

			var msg stun.Message
			msg.Raw = buf[:n]

			if err := msg.Decode(); err != nil {
				log.Println("Failed to decode STUN message:", err)
				continue
			}

			if msg.Type.Method == stun.MethodBinding && msg.Type.Class == stun.ClassRequest {
				resp, err := stun.Build(
					stun.TransactionID,
					stun.NewType(stun.MethodBinding, stun.ClassSuccessResponse),
					stun.XORMappedAddress{
						IP:   rAddr.IP,
						Port: rAddr.Port,
					},
				)
				if err != nil {
					log.Println("failed to build STUN response:", err)
					continue
				}

				copy(resp.TransactionID[:], msg.TransactionID[:])

				if _, err := conn.WriteToUDP(resp.Raw, rAddr); err != nil {
					log.Println("failed to send STUN response:", err)
				}
			} else {
				log.Println("Ignoring non-Binding Request message")
			}
		}
	}()
}

func parseICEServers(config conf.WebRTCICEServers) []webrtc.ICEServer {
	// * Note: The ClientOnly field is not directly used in webrtc.ICEServer
	var iceServers []webrtc.ICEServer

	for _, server := range config {
		iceServer := webrtc.ICEServer{
			URLs: []string{server.URL},
		}

		if server.Username != "" {
			iceServer.Username = server.Username
		}

		if server.Password != "" {
			iceServer.Credential = server.Password
			iceServer.CredentialType = webrtc.ICECredentialTypePassword
		}

		iceServers = append(iceServers, iceServer)
	}

	return iceServers
}

func ICEHandler(w http.ResponseWriter, r *http.Request, ICEServers *conf.WebRTCICEServers) {
	w.Header().Set("Content-Type", "application/json")

	candidateIPs := getAllLocalIPs()
	mappedServers := make([]map[string]interface{}, len(candidateIPs))

	for i, ip := range candidateIPs {
		stunServerURL := fmt.Sprintf("stun:%s:7009", ip)
		mapped := map[string]interface{}{
			"urls": stunServerURL,
		}
		mappedServers[i] = mapped
	}

	jsonResponse, err := json.Marshal(mappedServers)
	if err != nil {
		http.Error(w, "Unable to marshal ICE servers", http.StatusInternalServerError)
		return
	}

	w.Write(jsonResponse)
}

func EnableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func HandleBeaconStreamWebSocket(w http.ResponseWriter, r *http.Request, conf *conf.GPSConfig, ICEServers *conf.WebRTCICEServers, rawDataLog bool) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	client := &Client{conn: conn, conf: conf, ICEServers: ICEServers, rawDataLog: rawDataLog}
	handleSignaling(client)
}

func getAllLocalIPs() []string {
	var allIPs []string
	seenIPs := make(map[string]bool)

	// * Get ZeroTier assigned IP addresses (excluding loopback and IPv6)
	getZeroTierIPs := func() []string {
		var zeroTierIPs []string
		interfaces, err := net.Interfaces()
		if err != nil {
			fmt.Println("Error getting network interfaces:", err)
			return zeroTierIPs
		}

		for _, iface := range interfaces {
			if strings.HasPrefix(iface.Name, "zt") {
				addrs, err := iface.Addrs()
				if err != nil {
					fmt.Printf("Error getting addresses for interface %s: %v\n", iface.Name, err)
					continue
				}
				for _, addr := range addrs {
					if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil && !seenIPs[ipnet.IP.String()] {
						zeroTierIPs = append(zeroTierIPs, ipnet.IP.String())
						seenIPs[ipnet.IP.String()] = true
					}
				}
			}
		}
		return zeroTierIPs
	}

	// * Get local IP addresses (excluding loopback and IPv6)
	getLocalIPs := func() []string {
		var localIPs []string
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			fmt.Println("Error getting network interface addresses:", err)
			return localIPs
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil && !seenIPs[ipnet.IP.String()] {
				localIPs = append(localIPs, ipnet.IP.String())
				seenIPs[ipnet.IP.String()] = true
			}
		}
		return localIPs
	}

	zeroTierIPs := getZeroTierIPs()
	allIPs = append(allIPs, zeroTierIPs...)

	localIPs := getLocalIPs()
	allIPs = append(allIPs, localIPs...)

	return allIPs
}

func handleSignaling(client *Client) {
	settingEngine := webrtc.SettingEngine{}

	settingEngine.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeUDP4,
		webrtc.NetworkTypeTCP4,
		webrtc.NetworkTypeUDP6,
		webrtc.NetworkTypeTCP6,
	})

	api := webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))

	candidateIPs := getAllLocalIPs()
	fmt.Println("All local IPs found:", candidateIPs)

	var iceServers []webrtc.ICEServer
	for _, ip := range candidateIPs {
		stunServerURL := fmt.Sprintf("stun:%s:7009", ip)
		iceServers = append(iceServers, webrtc.ICEServer{URLs: []string{stunServerURL}})
	}

	config := webrtc.Configuration{
		ICEServers: iceServers,
	}

	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		log.Printf("Failed to create PeerConnection: %v", err)
		return
	}
	defer peerConnection.Close()

	peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
		log.Printf("OnICECandidate triggered. Candidate: %+v", c)
		if c == nil {
			log.Printf("OnICECandidate: Gathering complete (nil candidate).")
			return
		}
		log.Printf("Server generated candidate: %s", c.String())

		candidateJSON, err := json.Marshal(c.ToJSON())
		if err != nil {
			log.Printf("[ERROR] Failed to marshal ICE candidate: %v", err)
			return
		}
		message := Message{Candidate: string(candidateJSON)}
		log.Printf("[DEBUG] Attempting to send candidate message via WebSocket...")
		sendMessage(client, message)
		log.Printf("[DEBUG] Candidate message send attempt finished.")
	})

	dataChannel, err := peerConnection.CreateDataChannel("gpsData", nil)
	if err != nil {
		log.Printf("Failed to create DataChannel: %v", err)
		return
	}

	// * Register DataChannel handlers
	dataChannel.OnOpen(func() {
		log.Printf("DataChannel opened for client: %s", dataChannel.Label())
		addDataChannel(dataChannel)

		once.Do(func() {
			log.Printf("Starting data broadcast source (Protocol: %s)", client.conf.Protocol)
			switch client.conf.Protocol {
			case "ws":
				go broadcastGPSDataByWebsocket(fmt.Sprintf("%s://%s:%d", client.conf.Protocol, client.conf.IPAddress, client.conf.Port))
			case "tcp":
				go broadcastGPSDataByTCP(fmt.Sprintf("%s:%d", client.conf.IPAddress, client.conf.Port))
			case "udp":
				go broadcastGPSDataByUDP(fmt.Sprintf("%s:%d", client.conf.IPAddress, client.conf.Port), client.rawDataLog)
			default:
				log.Printf("No data source configured or invalid protocol: %s", client.conf.Protocol)
			}
		})
	})

	dataChannel.OnClose(func() {
		log.Printf("DataChannel closed for client: %s", dataChannel.Label())
		removeDataChannel(dataChannel)
	})

	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("PeerConnection State has changed: %s", state.String())
		// * Uncomment below for any additional cleanup, maybe useful for future
		// if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateDisconnected {
		// }
	})

	for {
		_, msgBytes, err := client.conn.ReadMessage()
		if err != nil {
			log.Printf("WebSocket read error: %v", err)

			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Unexpected WebSocket close error: %v", err)
			} else {
				log.Printf("WebSocket closed cleanly or as expected: %v", err)
			}
			return
		}

		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			log.Printf("Failed to unmarshal message: %v", err)
			continue
		}

		// * Handle SDP Offer from Client
		if msg.SDP != "" {
			var sdp webrtc.SessionDescription
			if err := json.Unmarshal([]byte(msg.SDP), &sdp); err != nil {
				log.Printf("Failed to unmarshal SDP Offer: %v", err)
				continue
			}

			log.Printf("Received SDP Offer from client")
			if err := peerConnection.SetRemoteDescription(sdp); err != nil {
				log.Printf("Failed to set remote description (Offer): %v", err)
				continue
			}

			// * Create SDP Answer
			answer, err := peerConnection.CreateAnswer(nil)
			if err != nil {
				log.Printf("Failed to create SDP Answer: %v", err)
				continue
			}

			// * Sets the LocalDescription, and starts our UDP listeners
			// ? Note: Pion gathers ICE candidates automatically after SetLocalDescription
			if err := peerConnection.SetLocalDescription(answer); err != nil {
				log.Printf("Failed to set local description (Answer): %v", err)
				continue
			}

			// * Send the Answer back to the client
			// * Candidates will be sent by the OnICECandidate callback
			log.Printf("Sending SDP Answer to client")
			answerJSON, err := json.Marshal(answer)
			if err != nil {
				log.Printf("Failed to marshal SDP Answer: %v", err)
				continue
			}
			response := Message{SDP: string(answerJSON)}
			sendMessage(client, response)
		}

		// * Handle ICE Candidate from Client
		if msg.Candidate != "" {
			var candidate webrtc.ICECandidateInit
			if err := json.Unmarshal([]byte(msg.Candidate), &candidate); err != nil {
				log.Printf("Failed to unmarshal remote ICE candidate: %v", err)
				continue
			}

			log.Printf("Received ICE Candidate from client: %s", candidate.Candidate)
			if err := peerConnection.AddICECandidate(candidate); err != nil {
				log.Printf("Failed to add received ICE candidate: %v", err)
			}
		}
	}
}

func addDataChannel(dc *webrtc.DataChannel) {
	dcMux.Lock()
	defer dcMux.Unlock()
	dataChannels = append(dataChannels, dc)
}

func removeDataChannel(dc *webrtc.DataChannel) {
	dcMux.Lock()
	defer dcMux.Unlock()
	for i, channel := range dataChannels {
		if channel == dc {
			dataChannels = append(dataChannels[:i], dataChannels[i+1:]...)
			break
		}
	}
}

func broadcastGPSDataByWebsocket(serverUrl string) {
	for {
		log.Printf("Connecting to external WebSocket server at %s", serverUrl)
		c, _, err := websocket.DefaultDialer.Dial(serverUrl, nil)
		if err != nil {
			log.Printf("Failed to connect to external WebSocket server: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Printf("Connected to external WebSocket server")

		for {
			_, msgBytes, err := c.ReadMessage()
			if err != nil {
				log.Printf("Error reading from external WebSocket: %v", err)
				c.Close()
				break
			}

			broadcastRawDataToDataChannels(msgBytes)
		}
	}
}

// Connects to a TCP server and expects the same gps data
//
// For dev context: Connection to the tcp server can be tested by running the nc_tcp_server_test.sh file and should see output like
//
//	DataChannel opened for client
//	Connecting to TCP server at 0.0.0.0:13370
//	Connected to TCP server at 0.0.0.0:13370
func broadcastGPSDataByTCP(serverUrl string) {
	for {
		log.Printf("Connecting to TCP server at %s", serverUrl)
		conn, err := net.Dial("tcp", serverUrl)
		if err != nil {
			log.Printf("Failed to connect to TCP server: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		log.Printf("Connected to TCP server at %s", serverUrl)
		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				log.Printf("Error reading from TCP server: %v", err)
				conn.Close()
				break
			}

			broadcastRawDataToDataChannels(line)
		}
	}
}

// Connects to a UDP server and expects the same gps data
//
// For dev context: Connection to the udp server can be tested by running the nc_udp_server_test.sh file and should see output like
// In UDP communication, there's no concept of a persistent connection like there is in TCP; instead, you send and receive messages to and from ports.
// net.ListenUDP function binds to a local address and port to receive UDP packets
//
//	DataChannel opened for client
//	Connecting to UDP server at 0.0.0.0:13370
//	Connected to UDP server at 0.0.0.0:13370
func broadcastGPSDataByUDP(serverUrl string, rawDataLog bool) {
	udpAddr, err := net.ResolveUDPAddr("udp", serverUrl)
	if err != nil {
		log.Printf("Failed to resolve UDP address: %v", err)
		return
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Printf("Failed to listen on UDP address: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("Listening on UDP address %s", serverUrl)

	buf := make([]byte, 4096)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Error reading from UDP: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if rawDataLog {
			rawData := buf[:n]
			log.Printf("Received raw data from %s: %s", addr.String(), string(rawData))
		}

		broadcastRawDataToDataChannels(buf[:n])
	}
}

// * Sends the raw JSON data to all connected WebRTC DataChannels.
func broadcastRawDataToDataChannels(data []byte) {
	dcMux.Lock()
	defer dcMux.Unlock()
	for _, dc := range dataChannels {
		if dc.ReadyState() == webrtc.DataChannelStateOpen {
			if err := dc.Send(data); err != nil {
				log.Printf("Failed to send data to DataChannel: %v", err)
			}
		}
	}
}

func sendMessage(client *Client, msg Message) {
	client.mu.Lock()
	defer client.mu.Unlock()

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		return
	}

	if err := client.conn.WriteMessage(websocket.TextMessage, msgBytes); err != nil {
		log.Printf("WebSocket write error: %v", err)
	}
}
