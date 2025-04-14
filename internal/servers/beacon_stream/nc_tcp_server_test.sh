while true; do
    nc -l -p 13370 | while read line; do
        echo "Client connected!"
        echo "Received: $line"
    done
done
