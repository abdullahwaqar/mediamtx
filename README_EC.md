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
