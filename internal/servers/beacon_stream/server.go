package beacon_stream

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

// Message defines the structure for signaling messages
type Message struct {
	SDP       string `json:"sdp,omitempty"`
	Candidate string `json:"candidate,omitempty"`
}

// type GPSData struct {
// 	Timestamp string  `json:"timestamp"`
// 	Latitude  float64 `json:"latitude"`
// 	Longitude float64 `json:"longitude"`
// }

// GPSData represents the structure of GPS data to be sent
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
		// Allow all origins for testing; restrict in production
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	dataChannels []*webrtc.DataChannel
	dcMux        sync.Mutex
	once         sync.Once // Ensures broadcastGPSData starts only once
)

// Client represents a connected WebSocket client
type Client struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func HandleBeaconStreamWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	client := &Client{conn: conn}
	handleSignaling(client)
}

func handleSignaling(client *Client) {
	api := webrtc.NewAPI()
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
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
			go broadcastGPSData()
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

func broadcastGPSData() {
	// * Connect to the external WebSocket server
	externalWSURL := "ws://3.91.74.146:8485"

	for {
		log.Printf("Connecting to external WebSocket server at %s", externalWSURL)
		c, _, err := websocket.DefaultDialer.Dial(externalWSURL, nil)
		if err != nil {
			log.Printf("Failed to connect to external WebSocket server: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Printf("Connected to external WebSocket server")
		// Start reading messages
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

// broadcastToDataChannels sends the GPSData to all connected WebRTC DataChannels
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
