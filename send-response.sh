#!/bin/bash

# Send response back to WhatsApp via the agent
# Usage: ./send-response.sh "Your response message here"

if [ -z "$1" ]; then
    echo "Usage: ./send-response.sh \"message\""
    exit 1
fi

# Write response to file that agent monitors
echo "$1" > .codebutler-response

echo "✅ Response queued for WhatsApp delivery"
