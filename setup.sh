#!/bin/bash

LINK_PATH="/usr/local/bin/tinyreload"

echo "Building binary..."
mkdir -p build/
go mod tidy
go build -ldflags="-s -w" -o build/tinyreload tinyreload.go

echo "Creating symlink.."
if [ -L "$LINKPATH" ] || [ -e "$LINK_PATH" ]; then
    sudo rm -f "$LINK_PATH"
fi


sudo ln -s $(pwd)/build/tinyreload "$LINK_PATH"
