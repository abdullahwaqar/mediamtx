"use strict";

(function () {
    // * * --------- Implementation for gps data channel ---------
    const SIGNALING_SERVER_URL = `ws://${window.location.hostname}:8080/gps-ws`;

    const statusDiv = document.getElementById("status");

    let gpsPc = null;
    let dataChannel = null;
    let remoteDescriptionSet = false;
    const iceCandidateQueue = [];

    function radiansToDegrees(radians) {
        return radians * (180 / Math.PI);
    }

    // * Initialize and draw the graph axes and grid
    function initializeGraph() {
        const canvas = document.getElementById('marker-graph');
        const ctx = canvas.getContext('2d');

        // * Clear the canvas
        ctx.clearRect(0, 0, canvas.width, canvas.height);

        // * Draw background
        ctx.fillStyle = '#2e2e2e';
        ctx.fillRect(0, 0, canvas.width, canvas.height);

        // * Draw grid lines
        ctx.strokeStyle = '#555555';
        ctx.lineWidth = 1;

        const gridSpacingDegrees = 10; // * Degrees between grid lines for better granularity
        const totalDegrees = 60; // * -30 to +30
        const pixelsPerDegree = canvas.width / totalDegrees; // * 400 / 60 ≈ 6.666 pixels per degree

        // *  Draw vertical grid lines (constant angleX)
        for (let i = -30; i <= 30; i += gridSpacingDegrees) {
            const x = (i + 30) * pixelsPerDegree;
            ctx.beginPath();
            ctx.moveTo(x, 0);
            ctx.lineTo(x, canvas.height);
            ctx.stroke();

            // * Optional: Add labels for angleX
            if (i !== 0) { // * Skip labeling the origin
                ctx.fillStyle = '#ffffff';
                ctx.font = '14px Arial';
                ctx.textAlign = 'center';
                ctx.fillText(`${i}°`, x, canvas.height / 2 + 20); // * Slight offset below X-axis
            }
        }

        // * Draw horizontal grid lines (constant angleY)
        for (let i = -30; i <= 30; i += gridSpacingDegrees) {
            const y = (30 - i) * pixelsPerDegree; // * Inverted Y-axis
            ctx.beginPath();
            ctx.moveTo(0, y);
            ctx.lineTo(canvas.width, y);
            ctx.stroke();

            if (i !== 0) {
                // * Skip labeling the origin
                ctx.fillStyle = '#ffffff';
                ctx.font = '14px Arial';
                ctx.textAlign = 'right';
                ctx.fillText(`${i}°`, canvas.width / 2 - 10, y + 5); // * Slight offset left of Y-axis
            }
        }

        // * Draw axes
        ctx.strokeStyle = '#ffffff';
        ctx.lineWidth = 1;

        // * X-Axis
        ctx.beginPath();
        ctx.moveTo(0, canvas.height / 2);
        ctx.lineTo(canvas.width, canvas.height / 2);
        ctx.stroke();

        // * Y-Axis
        ctx.beginPath();
        ctx.moveTo(canvas.width / 2, 0);
        ctx.lineTo(canvas.width / 2, canvas.height);
        ctx.stroke();
    }

    // * Function to plot a single point on the graph
    function plotPoint(angleX_deg, angleY_deg) {
        const canvas = document.getElementById('marker-graph');
        const ctx = canvas.getContext('2d');

        // * Coordinates mapping
        // * X: -30 to +30 mapped to 0 to canvas.width
        // * Y: +30 to -30 mapped to 0 to canvas.height (Inverted Y-axis)

        const totalDegrees = 60; // * -30 to +30
        const pixelsPerDegree = canvas.width / totalDegrees; // * 400 / 60 ≈ 6.666 pixels per degree

        const x = (angleX_deg + 30) * pixelsPerDegree;
        // * const y = (30 - angleY_deg) * pixelsPerDegree; // * Inverted Y-axis
        const y = (angleY_deg + 30) * pixelsPerDegree; // * Corrected inversion

        // * Draw the point
        ctx.fillStyle = 'red';
        ctx.beginPath();
        ctx.arc(x, y, 8, 0, 2 * Math.PI); // * Increased radius for better visibility
        ctx.fill();
    }

    // * Function to update Attitude HUD
    function updateAttitude(data) {
        const [yaw, pitch, roll] = data.values.map(val => parseFloat(val).toFixed(3));
        document.getElementById("yaw").textContent = yaw;
        document.getElementById("pitch").textContent = pitch;
        document.getElementById("roll").textContent = roll;
    }

    // * Function to update Marker HUD
    function updateMarker(data) {
        // * Convert angles from radians to degrees
        let angleX_deg = radiansToDegrees(parseFloat(data.angle_x));
        let angleY_deg = radiansToDegrees(parseFloat(data.angle_y));

        // * Clamp angles to -30 to +30 degrees based on FoV
        const clampedAngleX = Math.max(-30, Math.min(30, angleX_deg));
        const clampedAngleY = Math.max(-30, Math.min(30, angleY_deg));

        document.getElementById("marker-id").textContent = data.markerId;
        document.getElementById("angle-x").textContent = clampedAngleX.toFixed(3);
        document.getElementById("angle-y").textContent = clampedAngleY.toFixed(3);
        document.getElementById("distance").textContent = parseFloat(data.distance).toFixed(3);

        resetGraph();

        plotPoint(clampedAngleX, clampedAngleY);
    }

    // * Function to simulate receiving JSON data periodically
    function simulateDataStream() {
        setInterval(() => {
            // * Simulate updating Attitude
            updateAttitude({
                "type": "attitude",
                "values": [
                    (Math.random() * 360 - 180).toFixed(3), // * Yaw: -180 to +180
                    (Math.random() * 180 - 90).toFixed(3),  // * Pitch: -90 to +90
                    (Math.random() * 360 - 180).toFixed(3)  // * Roll: -180 to +180
                ],
                "timestamp": Date.now()
            });

            // * Simulate updating Marker
            const simulatedMarkerData = {
                "type": "marker",
                "markerId": Math.floor(Math.random() * 10),
                "angle_x": (Math.random() * 60 - 30) * (Math.PI / 180), // * Convert degrees to radians (-30° to +30°)
                "angle_y": (Math.random() * 60 - 30) * (Math.PI / 180), // * Convert degrees to radians (-30° to +30°)
                "distance": (Math.random() * 100).toFixed(3)
            };
            updateMarker(simulatedMarkerData);
        }, 2000); // * Update every 2 seconds
    }

    function resetGraph() {
        initializeGraph();
    }

    function render(data) {
        if (data.type === "marker") {
            updateMarker(data)
        }
        if (data.type === "attitude") {
            updateAttitude(data)
        }
    }

    // * Utility function to check if message is a valid JSON and matches expected structure
    const isValidJsonAndStructure = (message) => {
        try {
            const parsedMessage = JSON.parse(message);
            return parsedMessage;

        } catch (error) {
            return null;
        }
    };

    // * Utility function to log status
    const logStatus = (message) => {
        const data = isValidJsonAndStructure(message);

        if (data) {
            console.log('Valid JSON with expected structure:', data);
            statusDiv.textContent = message;

            render(data);
        } else {
            // * Just display the raw message
            console.log('Displaying message as-is:', message);
            statusDiv.textContent = message;
        }
    };

    let signalingSocket;
    let reconnectAttempts = 0;

    // * Set to Infinity for unlimited attempts
    const maxReconnectAttempts = Infinity;

    // * 2 seconds between reconnection attempts
    const reconnectInterval = 2000;

    // * Initialize the WebSocket connection
    function connectWebSocket() {
        signalingSocket = new WebSocket(SIGNALING_SERVER_URL);

        signalingSocket.onopen = () => {
            console.log("Connected to signaling server.");
            logStatus("Connected to signaling server.");
            // * Reset reconnection attempts on successful connection
            reconnectAttempts = 0;
            resetGraph();

            // ? For testing purposes; disable in production
            // * simulateDataStream();

            fetch(`http://${window.location.hostname}:8080/ice`)
                .then(response => {
                    return response.json();
                }).then(data => {
                    createPeerConnection(data);

                    createDataChannel();
                    createAndSendOffer();
                })
                .catch(error => {
                    console.error('Error fetching ICE servers:', error);
                });
        };

        signalingSocket.onerror = (error) => {
            console.error("Socket error:", error);
            logStatus("Socket error. Attempting to reconnect...");
            // * Close the socket to trigger the onclose event
            signalingSocket.close();
        };

        signalingSocket.onclose = (event) => {
            console.log("Socket closed:", event.reason);
            logStatus("Socket closed. Attempting to reconnect...");
            if (reconnectAttempts < maxReconnectAttempts) {
                setTimeout(() => {
                    reconnectAttempts++;
                    console.log(`Reconnection attempt ${reconnectAttempts}...`);
                    logStatus(`Reconnection attempt ${reconnectAttempts}...`);
                    connectWebSocket();
                }, reconnectInterval);
            } else {
                console.warn("Max reconnection attempts reached.");
                logStatus("Unable to reconnect to signaling server.");
            }
        };

        signalingSocket.onmessage = async (event) => {
            try {
                const msg = JSON.parse(event.data);
                console.log("Received message from server:", msg);

                if (msg.sdp) {
                    await handleRemoteSDP(msg.sdp);
                }

                if (msg.candidate) {
                    await handleRemoteCandidate(msg.candidate);
                }
            } catch (err) {
                console.error("Error parsing signaling message:", err);
            }
        };
    }

    connectWebSocket();

    // * Create RTCPeerConnection
    const createPeerConnection = (iceServers) => {
        gpsPc = new RTCPeerConnection({ iceServers });

        // * Handle ICE candidates
        gpsPc.onicecandidate = (event) => {
            if (event.candidate) {
                const message = {
                    candidate: JSON.stringify(event.candidate)
                };
                signalingSocket.send(JSON.stringify(message));
                console.log("Sent ICE candidate:", event.candidate);
            } else {
                console.log("ICE gathering completed.");
            }
        };

        // * Handle connection state changes
        gpsPc.oniceconnectionstatechange = () => {
            console.log(`ICE Connection State: ${gpsPc.iceConnectionState}`);
            logStatus(`ICE Connection State: ${gpsPc.iceConnectionState}`);
            if (gpsPc.iceConnectionState === "connected") {
                logStatus("Connected!");
            } else if (gpsPc.iceConnectionState === "disconnected" || gpsPc.iceConnectionState === "failed") {
                logStatus("Connection failed/disconnected.");
            }
        };

        // * Handle DataChannel messages if receiving channel
        gpsPc.ondatachannel = (event) => {
            const receiveChannel = event.channel;
            receiveChannel.onopen = () => {
                console.log("Receive DataChannel is open.");
                logStatus("DataChannel is open.");
            };

            receiveChannel.onmessage = async (event) => {
                if (event.data instanceof Blob) {
                    // * Convert Blob to text using arrayBuffer and TextDecoder
                    const arrayBuffer = await event.data.arrayBuffer();
                    const textData = new TextDecoder().decode(arrayBuffer);

                    // * Log the decoded message
                    console.log("Received message via DataChannel:", textData);
                    logStatus(textData);
                } else {
                    // * Handle string data directly
                    console.log("Received message via DataChannel:", event.data);
                    logStatus(new TextDecoder().decode(event.data))
                }
            };

            receiveChannel.onclose = () => {
                console.log("Receive DataChannel is closed.");
                logStatus("DataChannel is closed.");
            };
        };
    };

    // * Create DataChannel for sending messages
    const createDataChannel = () => {
        dataChannel = gpsPc.createDataChannel("gpsData");
        dataChannel.onopen = () => {
            console.log("DataChannel is open.");
            logStatus("DataChannel is open.");
        };

        dataChannel.onmessage = (event) => {
            console.log("Received message via DataChannel:", event.data);
            logStatus()
        };

        dataChannel.onclose = () => {
            console.log("DataChannel is closed.");
            logStatus("DataChannel is closed.");
        };
    };

    // * Create and send SDP offer
    const createAndSendOffer = async () => {
        try {
            const offer = await gpsPc.createOffer();
            await gpsPc.setLocalDescription(offer);
            logStatus("SDP offer created and set as local description.");

            const message = {
                sdp: JSON.stringify(gpsPc.localDescription)
            };
            signalingSocket.send(JSON.stringify(message));
            console.log("Sent SDP offer:", offer);
        } catch (err) {
            console.error("Error creating/sending SDP offer:", err);
        }
    };

    // * Handle remote SDP answer
    const handleRemoteSDP = async (sdp) => {
        try {
            const remoteDesc = new RTCSessionDescription(JSON.parse(sdp));
            await gpsPc.setRemoteDescription(remoteDesc);
            remoteDescriptionSet = true;
            console.log("Remote description set.");
            logStatus("Remote description set.");

            // * Add any buffered ICE candidates
            for (const candidate of iceCandidateQueue) {
                try {
                    await gpsPc.addIceCandidate(candidate);
                    console.log("Added buffered ICE candidate:", candidate);
                } catch (err) {
                    console.error("Error adding buffered ICE candidate:", err);
                }
            }
            // * * Clear the buffer
            iceCandidateQueue.length = 0;
        } catch (err) {
            console.error("Error setting remote description:", err);
        }
    };

    // * Handle remote ICE candidate
    const handleRemoteCandidate = async (candidateJSON) => {
        try {
            const candidate = new RTCIceCandidate(JSON.parse(candidateJSON));
            if (remoteDescriptionSet) {
                await gpsPc.addIceCandidate(candidate);
                console.log("Added ICE candidate:", candidate);
            } else {
                // * Buffer the candidate
                iceCandidateQueue.push(candidate);
                console.log("Buffered ICE candidate:", candidate);
            }
        } catch (err) {
            console.error("Error adding ICE candidate:", err);
        }
    };
})();