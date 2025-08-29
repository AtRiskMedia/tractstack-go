#!/bin/bash

# TractStack v2 Installation Script
# Can be run locally with CLI args or via curl | bash for interactive mode

set -euo pipefail

# Colors for output
GREEN='\033[32m'
BLUE='\033[34m'
YELLOW='\033[33m'
RED='\033[31m'
WHITE='\033[97m'
RESET='\033[0m'

# Show TractStack ASCII art
show_header() {
  echo -e "${GREEN}"
  echo ' ▄██▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄██▄▄▄▄▄▄▄██▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄ ▄▄▄'
  echo '  ██  ██ ██ ▀▀ ██ ██ ▀▀ ██ ██ ▀▀ ██ ▀▀ ██ ██ ▀▀ ██ ██'
  echo '  ██  ██▀█▄ ██▀██ ██ ▄▄ ██ ▀▀▀██ ██ ██▀██ ██ ▄▄ ██▀█▄'
  echo '  ██  ██ ██ ██▄██ ██▄██ ██ ██▄██ ██ ██▄██ ██▄██ ██ ██'
  echo '   ▀▀                   ▀▀       ▀▀             ▀▀ ▀▀▀'
  echo -e "${WHITE}"
  echo '  made by At Risk Media'
  echo -e "${RESET}"
  echo
}

# Check if running interactively (no arguments)
is_interactive() {
  [[ $# -eq 0 ]]
}

# Check prerequisites
check_prerequisites() {
  echo -e "${BLUE}Checking prerequisites...${RESET}"

  # Check Go
  if ! command -v go &>/dev/null; then
    echo -e "${RED}❌ Go is not installed. Please install Go 1.22+ first.${RESET}"
    exit 1
  fi

  GO_VERSION=$(go version | grep -o 'go[0-9]\+\.[0-9]\+' | sed 's/go//')
  echo -e "${GREEN}✅ Go ${GO_VERSION} found${RESET}"

  # Check Node.js/pnpm
  if ! command -v node &>/dev/null; then
    echo -e "${RED}❌ Node.js is not installed. Please install Node.js 18+ first.${RESET}"
    exit 1
  fi

  if ! command -v pnpm &>/dev/null; then
    echo -e "${YELLOW}⚠️  pnpm not found, installing...${RESET}"
    npm install -g pnpm
  fi

  NODE_VERSION=$(node --version)
  PNPM_VERSION=$(pnpm --version)
  echo -e "${GREEN}✅ Node.js ${NODE_VERSION} found${RESET}"
  echo -e "${GREEN}✅ pnpm ${PNPM_VERSION} found${RESET}"

  # Check Git
  if ! command -v git &>/dev/null; then
    echo -e "${RED}❌ Git is not installed. Please install Git first.${RESET}"
    exit 1
  fi

  echo -e "${GREEN}✅ Git found${RESET}"
  echo
}

# Interactive install choice
choose_install_type() {
  echo -e "${BLUE}Choose installation type:${RESET}"
  echo "1) Quick install (development setup)"
  echo "2) Production install"
  echo
  read -p "Enter choice [1-2]: " choice

  case $choice in
  1)
    return 0 # Quick install
    ;;
  2)
    echo -e "${YELLOW}"
    echo "🚧 Production install recipes coming soon!"
    echo -e "${RESET}"
    exit 0
    ;;
  *)
    echo -e "${RED}Invalid choice. Please enter 1 or 2.${RESET}"
    choose_install_type
    ;;
  esac
}

# CLI argument parsing
parse_cli_args() {
  while [[ $# -gt 0 ]]; do
    case $1 in
    --quick)
      INSTALL_TYPE="quick"
      shift
      ;;
    --production)
      echo -e "${YELLOW}🚧 Production install recipes coming soon!${RESET}"
      exit 0
      ;;
    --help | -h)
      show_usage
      exit 0
      ;;
    *)
      echo -e "${RED}Unknown option: $1${RESET}"
      show_usage
      exit 1
      ;;
    esac
  done

  if [[ -z "${INSTALL_TYPE:-}" ]]; then
    echo -e "${RED}Please specify --quick or --production${RESET}"
    show_usage
    exit 1
  fi
}

# Show usage for CLI mode
show_usage() {
  echo "TractStack v2 Installer"
  echo
  echo "Usage: $0 [OPTIONS]"
  echo
  echo "Options:"
  echo "  --quick       Quick development setup"
  echo "  --production  Production installation"
  echo "  --help, -h    Show this help message"
}

# Quick install implementation
quick_install() {
  USER=$(whoami)
  INSTALL_DIR="/home/$USER/t8k"

  echo -e "${BLUE}Starting TractStack quick installation...${RESET}"
  echo -e "Install directory: ${WHITE}$INSTALL_DIR${RESET}"
  echo

  # Check if install directory already exists
  if [[ -d "$INSTALL_DIR" ]]; then
    echo -e "${RED}❌ Directory $INSTALL_DIR already exists!${RESET}"
    echo "Please remove it first or choose a different location."
    exit 1
  fi

  echo -e "${BLUE}Step 1: Creating directory structure...${RESET}"
  mkdir -p "$INSTALL_DIR/src"
  echo -e "${GREEN}✅ Created $INSTALL_DIR/src${RESET}"

  echo -e "${BLUE}Step 2: Cloning TractStack Go backend...${RESET}"
  cd "$INSTALL_DIR/src"
  git clone https://github.com/AtRiskMedia/tractstack-go.git
  echo -e "${GREEN}✅ Cloned tractstack-go repository${RESET}"

  echo -e "${BLUE}Step 3: Building Go backend...${RESET}"
  cd tractstack-go
  echo "GO_BACKEND_PATH=/home/$USER/t8k/t8k-go-server/" >.env
  echo "GIN_MODE=release" >>.env
  go build -o tractstack-go ./cmd/tractstack-go
  echo -e "${GREEN}✅ Built tractstack-go binary${RESET}"

  echo -e "${BLUE}Step 4: Creating Astro frontend project...${RESET}"
  cd "$INSTALL_DIR/src"
  pnpm create astro@latest my-tractstack --template minimal --typescript strict --install
  echo -e "${GREEN}✅ Created Astro project${RESET}"

  echo -e "${BLUE}Step 5: Installing TractStack integration...${RESET}"
  cd my-tractstack
  pnpm add astro-tractstack@latest
  echo -e "${GREEN}✅ Installed astro-tractstack${RESET}"

  echo -e "${BLUE}Step 6: Pre-configuring backend path...${RESET}"
  echo "PRIVATE_GO_BACKEND_PATH=/home/$USER/t8k/t8k-go-server/" >.env
  echo -e "${GREEN}✅ Pre-populated .env file${RESET}"

  echo -e "${BLUE}Step 7: Running TractStack setup...${RESET}"
  npx create-tractstack
  echo -e "${GREEN}✅ TractStack configuration complete${RESET}"

  echo
  echo -e "${GREEN}🎉 TractStack installation complete!${RESET}"
  echo
  echo -e "${WHITE}To start your TractStack site:${RESET}"
  echo
  echo "1. Start your Go backend (in one terminal):"
  echo -e "   ${BLUE}cd $INSTALL_DIR/src/tractstack-go${RESET}"
  echo -e "   ${BLUE}./tractstack-go${RESET}"
  echo
  echo "2. Start your development server (in another terminal):"
  echo -e "   ${BLUE}cd $INSTALL_DIR/src/my-tractstack${RESET}"
  echo -e "   ${BLUE}pnpm dev${RESET}"
  echo
  echo -e "${WHITE}Your site will be available at:${RESET}"
  echo -e "   ${BLUE}http://localhost:4321${RESET}"
  echo
}

# Main execution
main() {
  show_header

  if is_interactive "$@"; then
    # Interactive mode (no arguments)
    check_prerequisites
    choose_install_type
    quick_install
  else
    # CLI mode with arguments
    parse_cli_args "$@"
    check_prerequisites
    if [[ "${INSTALL_TYPE}" == "quick" ]]; then
      quick_install
    fi
  fi
}

# Run main function with all arguments
main "$@"
