#!/bin/bash

# Ask a question via WhatsApp and wait for response
# Usage: ./ask-question.sh "question" "option1" "option2" "option3"

if [ "$#" -lt 3 ]; then
    echo "Usage: ./ask-question.sh \"question\" \"option1\" \"option2\" [option3...]"
    exit 1
fi

QUESTION="$1"
shift
OPTIONS=("$@")

# Write question to file
echo "$QUESTION" > .codebutler-question
for i in "${!OPTIONS[@]}"; do
    echo "$((i+1)). ${OPTIONS[$i]}" >> .codebutler-question
done

echo "❓ Question sent to WhatsApp..."

# Wait for answer (agent will write to .codebutler-answer)
rm -f .codebutler-answer
timeout=30
elapsed=0

while [ ! -f .codebutler-answer ] && [ $elapsed -lt $timeout ]; do
    sleep 1
    ((elapsed++))
done

if [ -f .codebutler-answer ]; then
    ANSWER=$(cat .codebutler-answer)
    rm -f .codebutler-answer .codebutler-question
    echo "✅ Answer received: $ANSWER"
    echo "$ANSWER"
else
    rm -f .codebutler-question
    echo "❌ Timeout waiting for answer"
    exit 1
fi
