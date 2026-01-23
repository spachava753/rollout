#!/bin/bash
# Test that task-b.txt exists with correct content
if [ -f "/app/task-b.txt" ]; then
    content=$(cat /app/task-b.txt)
    if [ "$content" = "Task B completed" ]; then
        echo "PASS: task-b.txt has correct content"
        echo 1 > /logs/verifier/reward.txt
        exit 0
    else
        echo "FAIL: task-b.txt has wrong content: $content"
        echo 0 > /logs/verifier/reward.txt
        exit 1
    fi
else
    echo "FAIL: task-b.txt not found"
    echo 0 > /logs/verifier/reward.txt
    exit 1
fi
