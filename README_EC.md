# GPS Server Implementation Documentation

## Introduction

This documentation provides a semi-technical overview of the new additions and points of interest in the GPS server implementation. The primary focus is on the `internal/servers/beacon_stream` folder, which includes the server implementation for GPS data communication using WebRTC and signaling via WebSockets. Additionally, it introduces three broadcasters for transmitting GPS data over different protocols.

## Folder Structure and Components

### Server Implementation

The server implementation is located in the `internal/servers/beacon_stream` directory. This server handles GPS data communication using WebRTC and utilizes WebSockets for signaling.

### Broadcasters

There are three broadcasters implemented:

- **broadcastGPSDataByWebsocket**
- **broadcastGPSDataByTCP**
- **broadcastGPSDataByUDP**

As the names suggest, these broadcasters connect to their respective server types, dictated by the configuration settings. They handle the transmission of GPS data over WebSocket, TCP, and UDP protocols, respectively.

## GPS Configuration

The GPS server's behavior is controlled via the `gpsConfig` section in the `mediamtx.yml` configuration file. If the `gpsConfig` key is not present, the GPS server will not be active, and the application will function as the default MediaMTX server without the extended GPS capabilities.

Below is the configuration snippet:

```yaml
###############################################
# GPS config. This holds the protocol, IP address, and port of the server that needs to connect to get the GPS data stream
# 'protocol' dictates the type of server to connect to in order to receive the data stream that will be sent over the data channel
gpsConfig:
  # Can be 'ws', 'tcp', or 'udp'
  protocol: udp

  # Specify server's IP address to connect to
  ipAddress: 0.0.0.0

  # Port number of the server
  port: 13370
```

### Configuration Parameters

- **protocol**: Defines the protocol used to connect to the GPS data source. Valid options are:
  - `ws` for WebSocket
  - `tcp` for TCP
  - `udp` for UDP

- **ipAddress**: The IP address of the server providing the GPS data stream.

- **port**: The port number on which the GPS data server is listening.

## Client-Side Implementation

The client-side code responsible for handling the GPS data is located at `internal/servers/webrtc/read_index.html`. A new script has been added to this HTML file, which:

- Establishes a peer connection.
- Opens a data channel named `"gps"`.
- Renders the GPS data output using plain JavaScript.

### Important Note

If you make any changes to the `read_index.html` file, you will need to rebuild the binary because MediaMTX includes this file in the binary and serves it directly from memory.

## Compilation Instructions

To compile the application and generate binaries for all supported platforms, you need to have Docker installed on your system. Follow these steps:

1. Ensure Docker is installed and running.
2. Navigate to the project's root directory.
3. Run the following command:

   ```bash
   make binaries
   ```

This command will create the binaries for all supported platforms.

---

Please ensure all configurations are set correctly before running the server to enable the GPS functionalities.
