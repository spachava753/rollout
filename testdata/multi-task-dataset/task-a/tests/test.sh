#!/bin/bash
# Test that task-a.txt exists with correct content
if [ -f "/app/task-a.txt" ]; then
    content=$(cat /app/task-a.txt)
    if [ "$content" = "Task A completed" ]; then
        echo "PASS: task-a.txt has correct content"
        echo 1 > /logs/verifier/reward.txt
        exit 0
    else
        echo "FAIL: task-a.txt has wrong content: $content"
        echo 0 > /logs/verifier/reward.txt
        exit 1
    fi
else
    echo "FAIL: task-a.txt not found"
    echo 0 > /logs/verifier/reward.txt
    exit 1
fi
