#!/bin/bash
set -e

FIRST_PORT=8080
LAST_PORT=8090
BASE=""
COOKIE_JAR="/tmp/goqrly_cookies.txt"
PID_FILE="/tmp/goqrly.pid"
EXTERNAL_SERVER=0

# Cleanup function
cleanup() {
    if [ "$EXTERNAL_SERVER" = "0" ] && [ -f "$PID_FILE" ]; then
        PID=$(cat "$PID_FILE")
        if kill -0 "$PID" 2>/dev/null; then
            echo "Stopping server (PID: $PID)..."
            kill "$PID" 2>/dev/null || true
            sleep 1
            kill -9 "$PID" 2>/dev/null || true
        fi
        rm -f "$PID_FILE"
    fi
}

trap cleanup EXIT

# FindRunningServer scans the port range to find a running goqrly server
find_running_server() {
    for port in $(seq $FIRST_PORT $LAST_PORT); do
        if curl -s --connect-timeout 1 "http://localhost:$port" > /dev/null 2>&1; then
            echo "$port"
            return 0
        fi
    done
    return 1
}

# Check if goqrly is already running
echo "Looking for goqrly server in ports $FIRST_PORT-$LAST_PORT..."
RUNNING_PORT=$(find_running_server || true)

if [ -n "$RUNNING_PORT" ]; then
    echo "Found server running on port $RUNNING_PORT - using existing server"
    BASE="http://localhost:$RUNNING_PORT"
    EXTERNAL_SERVER=1
elif [ -n "${PORT:-}" ]; then
    # Use PORT env var if set
    echo "Using PORT from environment: $PORT"
    BASE="http://localhost:$PORT"
    EXTERNAL_SERVER=1
else
    # Start our own server
    EXTERNAL_SERVER=0
    echo "Starting goqrly server..."
    ./goqrly &
    echo $! > "$PID_FILE"
    sleep 2

    # Verify server started and find which port it chose
    RUNNING_PORT=$(find_running_server || true)
    if [ -z "$RUNNING_PORT" ]; then
        echo "✗ Failed to start server"
        if [ -f "$PID_FILE" ]; then
            cat "$PID_FILE" | xargs kill 2>/dev/null || true
            rm -f "$PID_FILE"
        fi
        exit 1
    fi
    
    BASE="http://localhost:$RUNNING_PORT"
    echo "Server started on port $RUNNING_PORT (PID: $(cat "$PID_FILE"))"
fi
echo ""

# Helper function to extract CSRF token from HTML
extract_csrf() {
    local html="$1"
    echo "$html" | grep -oP 'name="csrf_token" value="\K[^"]+'
}

# Helper function to get CSRF token from URL
get_csrf() {
    local url="$1"
    curl -s "$url" | grep -oP 'name="csrf_token" value="\K[^"]+'
}

echo "=== goqrly tests ==="

# Test 1: Homepage loads
echo -n "Test 1: Homepage loads... "
if curl -s "$BASE" | grep -q "goqrly"; then
    echo "✓"
else
    echo "✗"
    exit 1
fi

# Test 2: Generate unprotected QR
echo -n "Test 2: Generate unprotected QR... "
HOMEPAGE=$(curl -s "$BASE")
CSRF_TOKEN=$(extract_csrf "$HOMEPAGE")
KEY=$(curl -s -X POST -d "text=https://hello.com&csrf_token=$CSRF_TOKEN" "$BASE/generate" -D - -o /dev/null | grep -i "^Location:" | tr -d '\r' | awk '{print $2}')
if [ -n "$KEY" ]; then
    echo "✓ (key: $KEY)"
else
    echo "✗"
    exit 1
fi

# Test 3: Unprotected QR shows on GET
echo -n "Test 3: Unprotected QR visible on GET... "
if curl -s "$BASE$KEY" | grep -q "data:image/png;base64,"; then
    echo "✓"
else
    echo "✗"
    exit 1
fi

# Test 4: Generate protected QR
echo -n "Test 4: Generate protected QR... "
HOMEPAGE=$(curl -s "$BASE")
CSRF_TOKEN=$(extract_csrf "$HOMEPAGE")
PROTECTED_KEY=$(curl -s -X POST -d "text=https://secret.com&password=secret123&csrf_token=$CSRF_TOKEN" "$BASE/generate" -D - -o /dev/null | grep -i "^Location:" | tr -d '\r' | awk '{print $2}')
if [ -n "$PROTECTED_KEY" ]; then
    echo "✓ (key: $PROTECTED_KEY)"
else
    echo "✗"
    exit 1
fi

# Test 5: Protected QR shows lock form on GET (no content leaked)
echo -n "Test 5: Protected QR lock form (no content leaked)... "
RESPONSE=$(curl -s "$BASE$PROTECTED_KEY")

if ! echo "$RESPONSE" | grep -q 'type="password"' || 
   ! echo "$RESPONSE" | grep -q "Confirm"; then
    echo "✗ (lock form missing)"
    exit 1
fi

if echo "$RESPONSE" | grep -q "data:image/png;base64,"; then
    echo "✗ (QR code leaked)"
    exit 1
fi

if echo "$RESPONSE" | grep -q "secret\.com" || echo "$RESPONSE" | grep -q "https://secret\.com"; then
    echo "✗ (entry text leaked)"
    exit 1
fi

echo "✓"

# Test 6: Wrong password
echo -n "Test 6: Wrong password rejected... "
CSRF_TOKEN=$(get_csrf "$BASE$PROTECTED_KEY")
if curl -s -X POST -d "password=wrong&csrf_token=$CSRF_TOKEN" "$BASE$PROTECTED_KEY" | grep -q "Wrong password"; then
    echo "✓"
else
    echo "✗"
    exit 1
fi

# Test 7: Correct password reveals QR
echo -n "Test 7: Correct password reveals QR... "
CSRF_TOKEN=$(get_csrf "$BASE$PROTECTED_KEY")
CONTENT=$(curl -s -X POST -d "password=secret123&csrf_token=$CSRF_TOKEN" "$BASE$PROTECTED_KEY")
if echo "$CONTENT" | grep -q "data:image/png;base64," && 
   echo "$CONTENT" | grep -q "secret\.com"; then
    echo "✓"
else
    echo "✗"
    exit 1
fi

# Test 8: Non-existent key redirects
echo -n "Test 8: Non-existent key redirects... "
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/nonexistent123")
if [ "$STATUS" = "302" ]; then
    echo "✓"
else
    echo "✗ (status: $STATUS)"
fi

echo ""
echo "=== All tests passed! ==="
