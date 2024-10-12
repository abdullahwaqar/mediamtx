package main

import (
	"encoding/json"
	"fmt"
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
		// Allow all origins for testing; restrict in production
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

// Client represents a connected WebSocket client
type Client struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func main() {
	http.HandleFunc("/ws", handleWebSocket)

	fmt.Println("Beacon Stream Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	client := &Client{conn: conn}

	// Handle signaling
	handleSignaling(client)
}

func handleSignaling(client *Client) {
	// Create a new WebRTC API object
	api := webrtc.NewAPI()

	// Prepare the configuration
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// Create a new PeerConnection
	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		log.Printf("Failed to create PeerConnection: %v", err)
		return
	}

	// Create a DataChannel
	dataChannel, err := peerConnection.CreateDataChannel("data", nil)
	if err != nil {
		log.Printf("Failed to create DataChannel: %v", err)
		return
	}

	// Generate a unique session ID (optional, for logging)
	sessionID := time.Now().UnixNano()

	// Register channel opening handling
	dataChannel.OnOpen(func() {
		log.Printf("DataChannel opened for session %d", sessionID)
		// Start sending GPS data every 2 seconds
		go sendGPSData(dataChannel, sessionID)
	})

	// Register channel message handling
	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		log.Printf("Received message from client: %s", string(msg.Data))
	})

	// Register ICE candidate handler
	peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			// ICE Gathering Complete
			return
		}
		candidateJSON, err := json.Marshal(c.ToJSON())
		if err != nil {
			log.Printf("Failed to marshal ICE candidate: %v", err)
			return
		}
		message := Message{
			Candidate: string(candidateJSON),
		}
		sendMessage(client, message)
	})

	// Handle incoming signaling messages
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
				// Create an answer
				answer, err := peerConnection.CreateAnswer(nil)
				if err != nil {
					log.Printf("Failed to create answer: %v", err)
					continue
				}

				// Set Local Description
				if err := peerConnection.SetLocalDescription(answer); err != nil {
					log.Printf("Failed to set local description: %v", err)
					continue
				}

				// Wait for ICE Gathering to complete
				gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

				// Send the answer back to the client
				<-gatherComplete

				localDesc := peerConnection.LocalDescription()
				localSDP, err := json.Marshal(localDesc)
				if err != nil {
					log.Printf("Failed to marshal local description: %v", err)
					continue
				}

				response := Message{
					SDP: string(localSDP),
				}

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

// sendMessage sends a signaling message to the client
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

// sendGPSData sends GPS data over the DataChannel every 2 seconds
func sendGPSData(dc *webrtc.DataChannel, sessionID int64) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if dc.ReadyState() != webrtc.DataChannelStateOpen {
			log.Printf("DataChannel is not open for session %d. Stopping GPS data sender.", sessionID)
			return
		}

		gpsData := generateGPSData()
		dataBytes, err := json.Marshal(gpsData)
		if err != nil {
			log.Printf("Failed to marshal GPS data: %v", err)
			continue
		}

		if err := dc.Send(dataBytes); err != nil {
			log.Printf("Failed to send GPS data: %v", err)
			continue
		}

		log.Printf("Sent GPS data to session %d: %s", sessionID, string(dataBytes))
	}
}

// generateGPSData creates dummy GPS data
func generateGPSData() GPSData {
	// For demonstration, we'll use random data or a simple pattern
	now := time.Now()
	latitude := 37.7749 + 0.0001*float64(now.Second()%60) // Simulate movement
	longitude := -122.4194 + 0.0001*float64(now.Second()%60)

	return GPSData{
		Timestamp: now.Format(time.RFC3339),
		Latitude:  latitude,
		Longitude: longitude,
	}
}
