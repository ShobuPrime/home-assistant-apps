#!/bin/bash
# Build script for AegisHA addon
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}=== AegisHA Addon Builder ===${NC}"

if [ ! -f "config.yaml" ] || [ ! -f "build.yaml" ] || [ ! -f "Dockerfile" ]; then
    echo -e "${RED}Error: run from the addon directory.${NC}"
    exit 1
fi

ADDON_SLUG=$(grep "^slug:" config.yaml | cut -d'"' -f2 | tr -d ' ')
ADDON_VERSION=$(grep "^version:" config.yaml | cut -d'"' -f2)

echo -e "Building ${GREEN}${ADDON_SLUG}${NC} version ${YELLOW}${ADDON_VERSION}${NC}"

ARCH=$(uname -m)
case $ARCH in
    aarch64) BUILD_ARCH="aarch64" ;;
    x86_64)  BUILD_ARCH="amd64"   ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

echo -e "Architecture: ${YELLOW}${BUILD_ARCH}${NC}"

BASE_IMAGE=$(grep -A2 "^build_from:" build.yaml | grep "  ${BUILD_ARCH}:" | awk '{print $2}')
if [ -z "$BASE_IMAGE" ]; then
    echo -e "${RED}Error: No base image found for architecture ${BUILD_ARCH}${NC}"
    exit 1
fi

echo -e "Base image: ${YELLOW}${BASE_IMAGE}${NC}"

IMAGE_NAME="local/${BUILD_ARCH}-addon-local_${ADDON_SLUG}"
IMAGE_TAG="${ADDON_VERSION}"
FULL_IMAGE="${IMAGE_NAME}:${IMAGE_TAG}"

echo ""
echo -e "${BLUE}Building Docker image...${NC}"
echo "Image: ${FULL_IMAGE}"
echo ""

docker build \
    --build-arg BUILD_FROM="${BASE_IMAGE}" \
    --build-arg BUILD_ARCH="${BUILD_ARCH}" \
    --build-arg BUILD_DATE="$(date -u +"%Y-%m-%dT%H:%M:%SZ")" \
    --build-arg BUILD_DESCRIPTION="Dynamic alarm system for Home Assistant with UniFi Protect integration" \
    --build-arg BUILD_NAME="${ADDON_SLUG}" \
    --build-arg BUILD_REF="$(git rev-parse --short HEAD 2>/dev/null || echo 'local')" \
    --build-arg BUILD_REPOSITORY="local" \
    --build-arg BUILD_VERSION="${ADDON_VERSION}" \
    -t "${FULL_IMAGE}" \
    .

echo ""
echo -e "${GREEN}Build successful!${NC}"
echo -e "Image: ${GREEN}${FULL_IMAGE}${NC}"
echo ""
echo "To test locally:"
echo -e "  ${YELLOW}docker run --rm -it -p 8099:8099 ${FULL_IMAGE}${NC}"
