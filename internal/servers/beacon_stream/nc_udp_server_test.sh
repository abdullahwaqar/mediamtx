#!/bin/bash

PORT=13370

echo "Starting UDP server on port $PORT..."

while true; do
    echo "Waiting for incoming UDP messages..."
    nc -u -l -p $PORT | while read -r line; do
        echo "Received message from client!"
        echo "Message: $line"
        
        response="Server received: $line"
        
        echo "$response" | nc -u -w1 localhost $PORT
    done
done
