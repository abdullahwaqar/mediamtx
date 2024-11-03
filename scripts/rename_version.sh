#!/bin/bash

# Check if the correct number of arguments are provided
if [ "$#" -ne 3 ]; then
    echo "Usage: $0 <directory> <old_version> <new_version>"
    exit 1
fi

# Assign arguments to variables
DIRECTORY="$1"
OLD_VERSION="$2"
NEW_VERSION="$3"

# Check if the provided directory exists
if [ ! -d "$DIRECTORY" ]; then
    echo "Error: Directory $DIRECTORY does not exist."
    exit 1
fi

# Loop through files with the old version in their name in the specified directory
for file in "$DIRECTORY"/mediamtx_${OLD_VERSION}_*; do
    # Check if the file exists
    if [ -e "$file" ]; then
        # Rename each file by replacing the old version with the new version
        mv "$file" "${file/$OLD_VERSION/$NEW_VERSION}"
        echo "Renamed $file to ${file/$OLD_VERSION/$NEW_VERSION}"
    else
        echo "No files found with version $OLD_VERSION in directory $DIRECTORY."
    fi
done

echo "Renaming process completed."
