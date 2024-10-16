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

// GPSData represents the structure of GPS data to be sent
type GPSData struct {
	Timestamp string  `json:"timestamp"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
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
		log.Printf("DataChannel opened")
		addDataChannel(dataChannel)

		// Start broadcastGPSData only once
		once.Do(func() {
			go broadcastGPSData()
		})
	})

	dataChannel.OnClose(func() {
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
	// * 100 Hz = 100 messages/second
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		gpsData := generateGPSData()
		dataBytes, err := json.Marshal(gpsData)
		if err != nil {
			log.Printf("Failed to marshal GPS data: %v", err)
			continue
		}

		dcMux.Lock()
		for _, dc := range dataChannels {
			if dc.ReadyState() == webrtc.DataChannelStateOpen {
				if err := dc.Send(dataBytes); err != nil {
					log.Printf("Failed to send GPS data: %v", err)
				}
			}
		}
		dcMux.Unlock()
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

func generateGPSData() GPSData {
	now := time.Now()
	latitude := 37.7749 + 0.0001*float64(now.Second()%60)
	longitude := -122.4194 + 0.0001*float64(now.Second()%60)

	return GPSData{
		Timestamp: now.Format(time.RFC3339),
		Latitude:  latitude,
		Longitude: longitude,
	}
}
