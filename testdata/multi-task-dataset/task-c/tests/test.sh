#!/bin/bash
# Test that task-c.txt exists with correct content
if [ -f "/app/task-c.txt" ]; then
    content=$(cat /app/task-c.txt)
    if [ "$content" = "Task C completed" ]; then
        echo "PASS: task-c.txt has correct content"
        echo 1 > /logs/verifier/reward.txt
        exit 0
    else
        echo "FAIL: task-c.txt has wrong content: $content"
        echo 0 > /logs/verifier/reward.txt
        exit 1
    fi
else
    echo "FAIL: task-c.txt not found"
    echo 0 > /logs/verifier/reward.txt
    exit 1
fi
