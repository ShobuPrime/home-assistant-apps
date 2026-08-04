#!/bin/bash
# Build script for MuninnDB app

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}=== MuninnDB App Builder ===${NC}"

# Check if we're in the right directory
if [ ! -f "config.yaml" ] || [ ! -f "build.yaml" ] || [ ! -f "Dockerfile" ]; then
    echo -e "${RED}Error: This script must be run from the app directory!${NC}"
    exit 1
fi

# Get app slug and version
APP_SLUG=$(grep "^slug:" config.yaml | cut -d'"' -f2 | tr -d ' ')
APP_VERSION=$(grep "^version:" config.yaml | cut -d'"' -f2)

echo -e "Building ${GREEN}${APP_SLUG}${NC} version ${YELLOW}${APP_VERSION}${NC}"

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64)
        BUILD_ARCH="amd64"
        ;;
    aarch64)
        BUILD_ARCH="aarch64"
        ;;
    armv7l)
        BUILD_ARCH="armv7"
        ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

echo -e "Architecture: ${YELLOW}${BUILD_ARCH}${NC}"

# Get base image from build.yaml
BASE_IMAGE=$(grep -A10 "^build_from:" build.yaml | grep "${BUILD_ARCH}:" | head -1 | awk '{print $2}')
if [ -z "$BASE_IMAGE" ]; then
    echo -e "${RED}Error: No base image found for architecture ${BUILD_ARCH}${NC}"
    exit 1
fi

echo -e "Base image: ${YELLOW}${BASE_IMAGE}${NC}"

# Get app version from build.yaml
MUNINNDB_VERSION=$(grep "MUNINNDB_VERSION:" build.yaml | cut -d':' -f2 | tr -d ' ')
echo -e "MuninnDB version: ${YELLOW}${MUNINNDB_VERSION}${NC}"

# Build image name
IMAGE_NAME="local/${BUILD_ARCH}-addon-local_${APP_SLUG}"
IMAGE_TAG="${APP_VERSION}"
FULL_IMAGE="${IMAGE_NAME}:${IMAGE_TAG}"

echo ""
echo -e "${BLUE}Building Docker image...${NC}"
echo "Image: ${FULL_IMAGE}"
echo ""

# Build the Docker image
docker build \
    --build-arg BUILD_FROM="${BASE_IMAGE}" \
    --build-arg BUILD_ARCH="${BUILD_ARCH}" \
    --build-arg BUILD_DATE="$(date -u +"%Y-%m-%dT%H:%M:%SZ")" \
    --build-arg BUILD_DESCRIPTION="MuninnDB for Home Assistant" \
    --build-arg BUILD_NAME="${APP_SLUG}" \
    --build-arg BUILD_REF="$(git rev-parse --short HEAD 2>/dev/null || echo 'local')" \
    --build-arg BUILD_REPOSITORY="local" \
    --build-arg BUILD_VERSION="${APP_VERSION}" \
    --build-arg MUNINNDB_VERSION="${MUNINNDB_VERSION}" \
    -t "${FULL_IMAGE}" \
    .

if [ $? -eq 0 ]; then
    echo ""
    echo -e "${GREEN}Build successful!${NC}"
    echo -e "Image: ${GREEN}${FULL_IMAGE}${NC}"
    echo ""
    echo "To test locally:"
    echo -e "  ${YELLOW}docker run --rm -it -p 8474:8474 -p 8475:8475 -p 8476:8476 -p 8477:8477 -p 8750:8750 ${FULL_IMAGE}${NC}"
else
    echo ""
    echo -e "${RED}Build failed!${NC}"
    exit 1
fi
