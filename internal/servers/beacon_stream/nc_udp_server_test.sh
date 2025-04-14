#!/bin/bash

SERVER_IP="127.0.0.1"
PORT=13370

while true; do
    read -p "Enter a message to send: " message
    echo "$message" | nc -u -w1 $SERVER_IP $PORT
    echo "Message sent to $SERVER_IP:$PORT"
done
