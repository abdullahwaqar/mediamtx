# GPS Server Implementation in MediaMTX

This documentation provides a semi-technical overview of the GPS server implementation within the MediaMTX project. The key components include server implementations for GPS data streaming using WebRTC, signaling via WebSockets, and configuration details to enable these features.

## Table of Contents

- [Introduction](#introduction)
- [Server Implementation](#server-implementation)
  - [Broadcasters](#broadcasters)
- [Configuration](#configuration)
  - [GPS Configuration Example](#gps-configuration-example)
- [Client-Side Implementation](#client-side-implementation)
- [Building and Running](#building-and-running)
  - [Using Docker](#using-docker)
  - [For Development](#for-development)
- [Important Notes](#important-notes)
- [Brief Architecture](#brief-architecture)

## Introduction

The point of interest and new addition to the MediaMTX project is the folder:

```
internal/servers/beacon_stream
```

This directory includes the server implementation for GPS data streaming that communicates using WebRTC and utilizes WebSockets for signaling.

## Server Implementation

The server implementation is designed to handle GPS data streaming through various protocols. It communicates using WebRTC for media transmission and WebSockets for signaling.

### Broadcasters

Within the `internal/servers/beacon_stream` folder, there are three broadcasters:

- **`broadcastGPSDataByWebsocket`**
- **`broadcastGPSDataByTCP`**
- **`broadcastGPSDataByUDP`**

As the names suggest, each broadcaster connects to the respective server type dictated by the configuration:

- **WebSocket**: Uses WebSockets to receive GPS data.
- **TCP**: Connects to a TCP server for GPS data.
- **UDP**: Connects to a UDP server for GPS data.

These broadcasters are controlled via the configuration file and are responsible for receiving GPS data from the specified server and protocol.

## Configuration

The GPS server functionality is configurable through the `mediamtx.yml` file. The configuration determines whether the GPS server is active and specifies the connection details.

If the `gpsConfig` key is absent in the configuration file, the GPS server will not be active, and MediaMTX will function as the default server without the extended GPS capability.

### GPS Configuration Example

Below is an example of the GPS configuration section in `mediamtx.yml`:

```yaml
###############################################
# GPS Configuration
# This holds the protocol, IP address, and port of the server that needs to connect to get the GPS data stream.
# The protocol dictates the type of server to connect to for the data stream that will be sent over the data channel.

gpsConfig:
  # Can be 'ws' (WebSocket), 'tcp', or 'udp'
  protocol: udp

  # Specify the server's IP address to connect to
  ipAddress: 0.0.0.0

  # Port number of the server
  port: 13370
```

- **`protocol`**: Specifies the protocol to use (`ws`, `tcp`, or `udp`).
- **`ipAddress`**: The IP address of the GPS data server.
- **`port`**: The port number of the GPS data server.

## Client-Side Implementation

The client-side implementation can be found in:

```
internal/servers/webrtc/read_index.html
```

A new script has been added to this HTML file that:

- Establishes a peer connection.
- Opens a data channel named **"gps"**.
- Renders the GPS output data on the client side.

The script is written in plain JavaScript.

**Note**: If you make any changes to this HTML file, you will need to rebuild the binary since MediaMTX includes this file in the binary and serves it directly from memory.

## Building and Running

### Using Docker

To compile the binaries for all supported platforms, you need to have Docker installed. Run the following command in the terminal:

```bash
make binaries
```

This command will create the binaries using Docker.

### For Development

For development purposes, you need to have the latest version of Go installed on your system. You can build and run MediaMTX by executing:

```bash
go build -o mediamtx . && ./mediamtx
```

This command builds the `mediamtx` binary and runs it.

## Important Notes

- **Rebuilding After HTML Changes**: If you modify the `read_index.html` file, you must rebuild the binary. MediaMTX embeds this file into the binary and serves it directly from memory.
- **Configuration Absence**: Without the `gpsConfig` key in `mediamtx.yml`, the GPS server features will be inactive, and MediaMTX will operate with its default capabilities.

---

### Brief Architecture

Modified MediaMTX by adding a **separate data channel** for transmitting GPS data, as MediaMTX typically only handles media streams (audio/video). In a WebRTC context, media streams require specific metadata, codec information, and a more complex handshake process to handle the transmission.

Since GPS data doesn’t need this level of complexity, and WebRTC channels are designed to handle distinct types of data, creating a dedicated data channel, kept the media and GPS data transmission separate, ensuring each type of data is handled appropriately without interfering with the other.

### 1. **Integration of WebRTC with a Separate Data Channel:**

- Leveraged **Pion WebRTC**, which is already included as a dependency in MediaMTX, to create a separate WebRTC **data channel**.
- This data channel is solely responsible for transmitting **GPS data** (yaw, pitch, roll, timestamp, and accuracy) to connected clients. It operates in parallel with the media channels, ensuring the system continues handling media while also allowing GPS data transmission.

### 2. **Non-blocking Server Setup:**

- The WebRTC server is created in a separate **Go routine**, ensuring that it runs in a **non-blocking** fashion. The GPS broadcasting server only starts if the appropriate GPS configuration is present (such as protocol, IP address, and port).
- This ensures that the GPS data channel does not interfere with the media streaming processes, keeping the system's core media-handling capabilities intact while extending its functionality.

### 3. **Simple Signaling Mechanism:**

- Kept the signaling mechanism as simple as possible to maintain a balance between functionality and complexity. WebRTC signaling is managed via **WebSocket**, handling **SDP offers**, **ICE candidates**, and peer connection establishment with minimal overhead.
- I used **WebSocket** for handling the signaling and managing WebRTC connection requests. Once the client sends an SDP offer, the server responds with an SDP answer, setting up the WebRTC connection.

### 4. **Data Channel Handling:**

- Once the WebRTC connection is established, I create a **data channel** with the label `"data"`. This channel is dedicated to sending GPS data, completely separate from the media streams.
- When the data channel opens, the server begins transmitting the GPS data. Made sure this process only starts **once** per connection to avoid redundant broadcasting and potential issues with multiple broadcast loops.

### 5. **Protocol-Specific GPS Broadcasting:**

- Depending on the GPS configuration, the server connects to external sources to retrieve GPS data using the protocol specified (WebSocket, TCP, or UDP).
  - **WebSocket:** The function `broadcastGPSDataByWebsocket` connects to an external WebSocket server and listens for GPS data to broadcast to WebRTC clients.
  - **TCP/UDP:** Similarly, `broadcastGPSDataByTCP` and `broadcastGPSDataByUDP` handle connections to TCP/UDP servers, read GPS data, and transmit it over the data channels.
- The GPS data is stored in the **GPSData** structure, which holds the values (yaw, pitch, roll), timestamp, and accuracy, and is transmitted to all connected WebRTC clients.

### 6. **Simple Synchronization and Concurrency:**

- To ensure smooth concurrent access to shared resources, used **sync.Mutex** for managing access to the data channels. The data channels are stored in a slice, and methods like `addDataChannel` and `removeDataChannel` ensure thread-safe modifications.
- Additionally, a **sync.Once** ensures that the GPS broadcasting logic is only initiated once for each session, keeping things simple and avoiding multiple unnecessary connections.

### 7. **Focus on Simplicity:**

- Throughout the modification process, made a conscious effort to keep the architecture **simple**. For example, I avoided adding unnecessary complexity in the signaling process and kept the configuration-driven GPS broadcasting flexible yet minimal.
- The WebRTC server and data broadcasting functions operate independently of the media transmission functions, ensuring that the core MediaMTX functionality remains unchanged while enabling seamless GPS data streaming.

### Key Aspects

- **Separation of concerns**: The data channel is isolated from the media streaming functionality, ensuring that the media transmission (audio/video) remains unaffected.
- **Protocol flexibility**: GPS data can be transmitted over WebSocket, TCP, or UDP, depending on the configuration.
- **Concurrency and simplicity**: I ensured non-blocking operation with simple synchronization techniques to keep the system lightweight and efficient.
