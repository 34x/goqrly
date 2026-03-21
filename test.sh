#!/bin/bash
set -e

PORT=${PORT:-8080}
BASE="http://localhost:$PORT"

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
KEY=$(curl -s -X POST -d "text=Hello" "$BASE/generate" -D - -o /dev/null | grep -i "^Location:" | tr -d '\r' | awk '{print $2}')
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
PROTECTED_KEY=$(curl -s -X POST -d "text=Secret&password=secret123" "$BASE/generate" -D - -o /dev/null | grep -i "^Location:" | tr -d '\r' | awk '{print $2}')
if [ -n "$PROTECTED_KEY" ]; then
    echo "✓ (key: $PROTECTED_KEY)"
else
    echo "✗"
    exit 1
fi

# Test 5: Protected QR shows lock form on GET (no QR in HTML)
echo -n "Test 5: Protected QR lock form (no QR in source)... "
RESPONSE=$(curl -s "$BASE$PROTECTED_KEY")
if echo "$RESPONSE" | grep -q "Unlock" && ! echo "$RESPONSE" | grep -q "data:image/png;base64,"; then
    echo "✓"
else
    echo "✗"
    exit 1
fi

# Test 6: Wrong password
echo -n "Test 6: Wrong password rejected... "
if curl -s -X POST -d "password=wrong" "$BASE$PROTECTED_KEY" | grep -q "Wrong password"; then
    echo "✓"
else
    echo "✗"
    exit 1
fi

# Test 7: Correct password reveals QR
echo -n "Test 7: Correct password reveals QR... "
if curl -s -X POST -d "password=secret123" "$BASE$PROTECTED_KEY" | grep -q "data:image/png;base64,"; then
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

# Test 9: QR endpoint for unprotected
echo -n "Test 9: QR endpoint returns PNG... "
if curl -s "$BASE/qr$KEY" | file - | grep -q "PNG"; then
    echo "✓"
else
    echo "✗"
    exit 1
fi

# Test 10: QR endpoint for protected redirects
echo -n "Test 10: QR endpoint for protected redirects... "
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/qr$PROTECTED_KEY")
if [ "$STATUS" = "302" ]; then
    echo "✓"
else
    echo "✗ (status: $STATUS)"
fi

echo ""
echo "=== All tests passed! ==="
