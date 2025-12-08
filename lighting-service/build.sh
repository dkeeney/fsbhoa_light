#!/bin/bash

echo "--- Building FSBHOA Lighting Service ---"

# Ensure all dependencies are present and clean up unused ones
echo "Tidying dependencies..."
go mod tidy

# Compile the application for your Raspberry Pi's architecture
echo "Compiling..."
go build -o lighting-service .

echo ""
echo "✅ Build complete!"
echo "You can now enable and start the service with:"
echo "sudo systemctl restart fsbhoa-lighting.service"

