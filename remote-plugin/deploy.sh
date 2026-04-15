#!/bin/bash
# Remove old zip if it exists
rm -f fsbhoa-remote-lighting.zip

# Zip the current directory contents
# -r for recursive (includes assets folder)
zip -r fsbhoa-remote-lighting.zip . -x "*.git*"

echo "On the PC use cmd:"
echo "scp fsbhoa@access.fsbhoa.com:~/fsbhoa_light/remote-plugin/fsbhoa-remote-lighting.zip %USERPROFILE%/Downloads/"


