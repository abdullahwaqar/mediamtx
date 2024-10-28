package beacon_stream

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

// * Message defines the structure for signaling messages
type Message struct {
	SDP       string `json:"sdp,omitempty"`
	Candidate string `json:"candidate,omitempty"`
}

// * GPSData represents the structure of GPS data to be sent
type GPSData struct {
	// * Yaw, Pitch, Roll values
	Values    [3]float64 `json:"values"`
	Timestamp int64      `json:"timestamp"`
	Accuracy  int        `json:"accuracy"`
}

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		// * Allow all origins for testing;
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	dataChannels []*webrtc.DataChannel
	dcMux        sync.Mutex
	once         sync.Once // Ensures broadcastGPSData starts only once
)

// * Client represents a connected WebSocket client
type Client struct {
	conn *websocket.Conn
	mu   sync.Mutex

	ICEServers *conf.WebRTCICEServers
	conf       *conf.GPSConfig
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
	mappedServers := make([]map[string]interface{}, len(*ICEServers))

	for i, server := range *ICEServers {
		mapped := map[string]interface{}{
			"urls": server.URL,
		}

		mapped["username"] = server.Username
		mapped["credential"] = server.Password
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

func HandleBeaconStreamWebSocket(w http.ResponseWriter, r *http.Request, conf *conf.GPSConfig, ICEServers *conf.WebRTCICEServers) {
	fmt.Printf("%#v\n", ICEServers)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	client := &Client{conn: conn, conf: conf, ICEServers: ICEServers}
	handleSignaling(client)
}

func handleSignaling(client *Client) {
	api := webrtc.NewAPI()
	config := webrtc.Configuration{
		ICEServers: parseICEServers(*client.ICEServers),
	}

	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		log.Printf("Failed to create PeerConnection: %v", err)
		return
	}

	dataChannel, err := peerConnection.CreateDataChannel("data", nil)
	if err != nil {
		log.Printf("Failed to create DataChannel: %v", err)
		return
	}

	dataChannel.OnOpen(func() {
		log.Printf("DataChannel opened for client")
		addDataChannel(dataChannel)

		// * Start the broadcaster only once
		once.Do(func() {
			switch client.conf.Protocol {
			case "ws":
				go broadcastGPSDataByWebsocket(fmt.Sprintf("%s://%s:%d", client.conf.Protocol, client.conf.IPAddress, client.conf.Port))
			case "tcp":
				go broadcastGPSDataByTCP(fmt.Sprintf("%s:%d", client.conf.IPAddress, client.conf.Port))
			case "udp":
				go broadcastGPSDataByUDP(fmt.Sprintf("%s:%d", client.conf.IPAddress, client.conf.Port))
			default:
				// * Ideally should never see this
				log.Printf("No data source found")
			}
		})
	})

	dataChannel.OnClose(func() {
		log.Printf("DataChannel closed for client")
		removeDataChannel(dataChannel)
	})

	peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		candidateJSON, err := json.Marshal(c.ToJSON())
		if err != nil {
			log.Printf("Failed to marshal ICE candidate: %v", err)
			return
		}
		message := Message{Candidate: string(candidateJSON)}
		sendMessage(client, message)
	})

	for {
		_, msgBytes, err := client.conn.ReadMessage()
		if err != nil {
			log.Printf("WebSocket read error: %v", err)
			return
		}

		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			log.Printf("Failed to unmarshal message: %v", err)
			continue
		}

		if msg.SDP != "" {
			var sdp webrtc.SessionDescription
			if err := json.Unmarshal([]byte(msg.SDP), &sdp); err != nil {
				log.Printf("Failed to unmarshal SDP: %v", err)
				continue
			}

			if err := peerConnection.SetRemoteDescription(sdp); err != nil {
				log.Printf("Failed to set remote description: %v", err)
				continue
			}

			if sdp.Type == webrtc.SDPTypeOffer {
				answer, err := peerConnection.CreateAnswer(nil)
				if err != nil {
					log.Printf("Failed to create answer: %v", err)
					continue
				}

				if err := peerConnection.SetLocalDescription(answer); err != nil {
					log.Printf("Failed to set local description: %v", err)
					continue
				}

				gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
				<-gatherComplete

				localDesc := peerConnection.LocalDescription()
				localSDP, err := json.Marshal(localDesc)
				if err != nil {
					log.Printf("Failed to marshal local description: %v", err)
					continue
				}

				response := Message{SDP: string(localSDP)}
				sendMessage(client, response)
			}
		}

		if msg.Candidate != "" {
			var candidate webrtc.ICECandidateInit
			if err := json.Unmarshal([]byte(msg.Candidate), &candidate); err != nil {
				log.Printf("Failed to unmarshal ICE candidate: %v", err)
				continue
			}

			if err := peerConnection.AddICECandidate(candidate); err != nil {
				log.Printf("Failed to add ICE candidate: %v", err)
				continue
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

		// * Start reading messages
		for {
			_, msgBytes, err := c.ReadMessage()
			if err != nil {
				log.Printf("Error reading from external WebSocket: %v", err)
				c.Close()
				break
			}

			var gpsData GPSData
			if err := json.Unmarshal(msgBytes, &gpsData); err != nil {
				log.Printf("Failed to unmarshal GPS data: %v", err)
				continue
			}

			// * Broadcast the received GPS data to all DataChannels
			broadcastToDataChannels(gpsData)
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

			var gpsData GPSData
			if err := json.Unmarshal(line, &gpsData); err != nil {
				log.Printf("Failed to unmarshal GPS data from TCP: %v", err)
				continue
			}
			broadcastToDataChannels(gpsData)
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
func broadcastGPSDataByUDP(serverUrl string) {
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

	buf := make([]byte, 1024)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Error reading from UDP: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		var gpsData GPSData
		if err := json.Unmarshal(buf[:n], &gpsData); err != nil {
			log.Printf("Failed to unmarshal GPS data from UDP: %v", err)
			continue
		}

		broadcastToDataChannels(gpsData)
	}
}

// * Sends the GPSData to all connected WebRTC DataChannels
func broadcastToDataChannels(gpsData GPSData) {
	dataBytes, err := json.Marshal(gpsData)
	if err != nil {
		log.Printf("Failed to marshal GPS data: %v", err)
		return
	}

	dcMux.Lock()
	defer dcMux.Unlock()
	for _, dc := range dataChannels {
		if dc.ReadyState() == webrtc.DataChannelStateOpen {
			if err := dc.Send(dataBytes); err != nil {
				log.Printf("Failed to send GPS data: %v", err)
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
