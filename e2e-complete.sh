#!/bin/bash

# SkyOps Complete End-to-End Test Suite
# Tests: Express Auth Server + FastAPI GPU Server + Go Agent + Next.js Client

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

# Configuration
EXPRESS_PORT=6000    # Express server for auth (actual port from logs)
FASTAPI_PORT=8000    # FastAPI server for GPU network API
CLIENT_PORT=3000     # Next.js client
AGENT_PORT=8080      # Agent callback port

echo -e "${BLUE}🚀 SkyOps Complete End-to-End Test Suite${NC}"
echo "=============================================="
echo -e "Testing: ${PURPLE}Express Auth${NC} + ${YELLOW}FastAPI GPU${NC} + ${GREEN}Go Agent${NC} + ${BLUE}Next.js Client${NC}"

# Function to check if port is in use
check_port() {
    local port=$1
    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
        return 0
    else
        return 1
    fi
}

# Function to wait for service to be ready
wait_for_service() {
    local url=$1
    local name=$2
    local max_attempts=30
    local attempt=1
    
    echo -e "${YELLOW}⏳ Waiting for $name to be ready...${NC}"
    
    while [ $attempt -le $max_attempts ]; do
        if curl -s "$url" >/dev/null 2>&1; then
            echo -e "${GREEN}✅ $name is ready!${NC}"
            return 0
        fi
        echo -n "."
        sleep 2
        ((attempt++))
    done
    
    echo -e "${RED}❌ $name failed to start within $((max_attempts * 2)) seconds${NC}"
    return 1
}

# Function to cleanup processes
cleanup() {
    echo -e "\n${YELLOW}🧹 Cleaning up all services...${NC}"
    
    # Kill background processes
    jobs -p | xargs -r kill 2>/dev/null || true
    
    # Kill any remaining processes on our ports
    pkill -f "node.*server" 2>/dev/null || true
    pkill -f "next.*dev" 2>/dev/null || true
    pkill -f "uvicorn.*main" 2>/dev/null || true
    pkill -f "python.*main.py" 2>/dev/null || true
    
    echo -e "${GREEN}✅ Cleanup complete${NC}"
}

# Set up cleanup trap
trap cleanup EXIT INT TERM

echo -e "\n${BLUE}📋 Pre-flight checks${NC}"
echo "===================="

# Check if required directories exist
if [ ! -d "/home/ovd/projects/skyops/server" ]; then
    echo -e "${RED}❌ Express server directory not found${NC}"
    exit 1
fi

if [ ! -d "/home/ovd/projects/skyops/gpu/server" ]; then
    echo -e "${RED}❌ FastAPI server directory not found${NC}"
    exit 1
fi

if [ ! -d "/home/ovd/projects/skyops/client" ]; then
    echo -e "${RED}❌ Client directory not found${NC}"
    exit 1
fi

if [ ! -f "skyops" ]; then
    echo -e "${YELLOW}⚠️  Agent binary not found, building...${NC}"
    make dev
fi

echo -e "${GREEN}✅ All components found${NC}"

# Check for port conflicts
echo -e "\n${BLUE}🔍 Checking port availability${NC}"
for port in $EXPRESS_PORT $FASTAPI_PORT $CLIENT_PORT; do
    if check_port $port; then
        echo -e "${RED}❌ Port $port is already in use${NC}"
        echo -e "${YELLOW}💡 Run: sudo lsof -i :$port to see what's using it${NC}"
        exit 1
    fi
done

echo -e "${GREEN}✅ All ports are available${NC}"

echo -e "\n${BLUE}🚀 Starting all services${NC}"
echo "========================="

# Start the Express server (Auth)
echo -e "${PURPLE}🔐 Starting Express Auth server on port $EXPRESS_PORT...${NC}"
cd /home/ovd/projects/skyops/server
npm run start > ../gpu/agent-go/express.log 2>&1 &
EXPRESS_PID=$!
echo -e "${PURPLE}   Express PID: $EXPRESS_PID${NC}"
cd - >/dev/null

# Start the FastAPI server (GPU Network API)
echo -e "${YELLOW}⚡ Starting FastAPI GPU server on port $FASTAPI_PORT...${NC}"
cd /home/ovd/projects/skyops/gpu/server
chmod +x start.sh
./start.sh > ../../gpu/agent-go/fastapi.log 2>&1 &
FASTAPI_PID=$!
echo -e "${YELLOW}   FastAPI PID: $FASTAPI_PID${NC}"
cd - >/dev/null

# Start the Next.js client
echo -e "${BLUE}🌐 Starting Next.js client on port $CLIENT_PORT...${NC}"
cd /home/ovd/projects/skyops/client
npm run dev > ../gpu/agent-go/client.log 2>&1 &
CLIENT_PID=$!
echo -e "${BLUE}   Client PID: $CLIENT_PID${NC}"
cd - >/dev/null

# Wait for services to be ready
echo -e "\n${BLUE}⏳ Waiting for all services to start${NC}"
echo "===================================="

# Wait for Express server
if ! wait_for_service "http://localhost:$EXPRESS_PORT" "Express Auth Server"; then
    echo -e "${RED}❌ Express server failed to start${NC}"
    echo -e "${YELLOW}📋 Express logs:${NC}"
    tail -n 20 express.log 2>/dev/null || echo "No logs available"
    exit 1
fi

# Wait for FastAPI server
if ! wait_for_service "http://localhost:$FASTAPI_PORT/docs" "FastAPI GPU Server"; then
    echo -e "${RED}❌ FastAPI server failed to start${NC}"
    echo -e "${YELLOW}📋 FastAPI logs:${NC}"
    tail -n 20 fastapi.log 2>/dev/null || echo "No logs available"
    exit 1
fi

# Wait for Client
if ! wait_for_service "http://localhost:$CLIENT_PORT" "Next.js Client"; then
    echo -e "${RED}❌ Client failed to start${NC}"
    echo -e "${YELLOW}📋 Client logs:${NC}"
    tail -n 20 client.log 2>/dev/null || echo "No logs available"
    exit 1
fi

echo -e "\n${BLUE}🤖 Testing Go Agent${NC}"
echo "===================="

# Test agent commands
echo -e "${YELLOW}📋 Testing agent version...${NC}"
./skyops version

echo -e "\n${YELLOW}📋 Testing agent help...${NC}"
./skyops help

echo -e "\n${YELLOW}📊 Testing agent status (should show not configured)...${NC}"
./skyops status

echo -e "\n${BLUE}🧪 API Tests${NC}"
echo "============="

# Test Express server
echo -e "${PURPLE}🔐 Testing Express Auth server...${NC}"
if curl -s "http://localhost:$EXPRESS_PORT" >/dev/null; then
    echo -e "${GREEN}✅ Express server is accessible${NC}"
else
    echo -e "${RED}❌ Express server is not accessible${NC}"
fi

# Test FastAPI server health
echo -e "\n${YELLOW}⚡ Testing FastAPI GPU server...${NC}"
if curl -s "http://localhost:$FASTAPI_PORT/docs" >/dev/null; then
    echo -e "${GREEN}✅ FastAPI server is accessible${NC}"
    echo -e "${GREEN}✅ API docs available at: http://localhost:$FASTAPI_PORT/docs${NC}"
else
    echo -e "${RED}❌ FastAPI server is not accessible${NC}"
fi

# Test client accessibility
echo -e "\n${BLUE}🌐 Testing Next.js client...${NC}"
if curl -s "http://localhost:$CLIENT_PORT" >/dev/null; then
    echo -e "${GREEN}✅ Client is accessible${NC}"
else
    echo -e "${RED}❌ Client is not accessible${NC}"
fi

echo -e "\n${GREEN}🎉 All services are running successfully!${NC}"
echo -e "\n${BLUE}📊 Service Dashboard${NC}"
echo "===================="
echo -e "🔐 Express Auth Server: ${GREEN}http://localhost:$EXPRESS_PORT${NC}"
echo -e "⚡ FastAPI GPU Server:   ${GREEN}http://localhost:$FASTAPI_PORT${NC}"
echo -e "📚 API Documentation:   ${GREEN}http://localhost:$FASTAPI_PORT/docs${NC}"
echo -e "🌐 Next.js Client:      ${GREEN}http://localhost:$CLIENT_PORT${NC}"
echo -e "🤖 Go Agent:            ${GREEN}./skyops [command]${NC}"

echo -e "\n${BLUE}🎮 Manual Testing Guide${NC}"
echo "======================="
echo "1. Open the client: http://localhost:$CLIENT_PORT"
echo "2. Try authentication flow (Express server)"
echo "3. Test API endpoints: http://localhost:$FASTAPI_PORT/docs"
echo "4. Test agent commands:"
echo "   ./skyops register    # Register with network"
echo "   ./skyops start       # Start agent daemon"
echo "   ./skyops status      # Check status"
echo ""
echo -e "${YELLOW}📋 Logs are available in:${NC}"
echo "   express.log  - Express server logs"
echo "   fastapi.log  - FastAPI server logs"  
echo "   client.log   - Next.js client logs"
echo ""
echo -e "${RED}Press Ctrl+C to stop all services${NC}"

# Keep script running
while true; do
    sleep 1
done
