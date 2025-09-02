#!/bin/bash

# TractStack v2 Installation Script
# Can be run locally with CLI args or via curl | bash for interactive mode
#
# https://github.com/AtRiskMedia/tractstack-go

set -euo pipefail

# Colors for output
GREEN='\033[32m'
BLUE='\033[34m'
YELLOW='\033[33m'
RED='\033[31m'
WHITE='\033[97m'
RESET='\033[0m'

# Global variables
DRY_RUN=false
INSTALL_TYPE=""
DOMAIN=""
NON_INTERACTIVE=false
SITE_ID=""
OS=""
PACKAGE_MANAGER=""
CURRENT_USER=""
PRODUCTION_AVAILABLE=false
MISSING_PRODUCTION_DEPS=()
ALLOCATED_GO_PORT=""
ALLOCATED_ASTRO_PORT=""
PORTS_CONFIG_FILE="/home/t8k/etc/t8k-ports.conf"

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

cleanup_lock() {
  if [[ -n "${LOCK_FILE:-}" ]]; then
    rm -f "$LOCK_FILE"
  fi
}

# Check for installation lock
LOCK_FILE="/tmp/t8k-install.lock"
if [[ -f "$LOCK_FILE" ]]; then
  echo -e "${RED}❌ Another TractStack production installation is already running${RESET}"
  echo "Lock file: $LOCK_FILE"
  exit 1
fi
touch "$LOCK_FILE"

# Check if running interactively (no arguments)
is_interactive() {
  [[ $# -eq 0 ]]
}

# Detect package manager and OS
detect_os_and_package_manager() {
  if [[ "$OSTYPE" == "darwin"* ]]; then
    OS="macos"
    if command -v brew &>/dev/null; then
      PACKAGE_MANAGER="brew"
    else
      PACKAGE_MANAGER="none"
    fi
  elif [[ -f /etc/os-release ]]; then
    . /etc/os-release
    case $ID in
    ubuntu | debian)
      OS="debian"
      PACKAGE_MANAGER="apt"
      ;;
    fedora | rhel | centos)
      OS="redhat"
      PACKAGE_MANAGER="dnf"
      ;;
    arch | manjaro)
      OS="arch"
      PACKAGE_MANAGER="pacman"
      ;;
    opensuse* | suse)
      OS="opensuse"
      PACKAGE_MANAGER="zypper"
      ;;
    alpine)
      OS="alpine"
      PACKAGE_MANAGER="apk"
      ;;
    *)
      OS="unknown"
      PACKAGE_MANAGER="unknown"
      ;;
    esac
  else
    OS="unknown"
    PACKAGE_MANAGER="unknown"
  fi
}

# Show installation instructions based on detected OS
show_install_instructions() {
  local dep=$1
  case $dep in
  nginx)
    case $PACKAGE_MANAGER in
    apt) echo -e "  ${BLUE}nginx${RESET}: sudo apt update && sudo apt install nginx" ;;
    dnf) echo -e "  ${BLUE}nginx${RESET}: sudo dnf install nginx" ;;
    pacman) echo -e "  ${BLUE}nginx${RESET}: sudo pacman -S nginx" ;;
    zypper) echo -e "  ${BLUE}nginx${RESET}: sudo zypper install nginx" ;;
    apk) echo -e "  ${BLUE}nginx${RESET}: sudo apk add nginx" ;;
    brew) echo -e "  ${BLUE}nginx${RESET}: brew install nginx" ;;
    *) echo -e "  ${BLUE}nginx${RESET}: Install nginx using your package manager" ;;
    esac
    ;;
  pm2)
    echo -e "  ${BLUE}pm2${RESET}: npm install -g pm2"
    ;;
  sqlite3)
    case $PACKAGE_MANAGER in
    apt) echo -e "  ${BLUE}sqlite3${RESET}: sudo apt update && sudo apt install sqlite3" ;;
    dnf) echo -e "  ${BLUE}sqlite3${RESET}: sudo dnf install sqlite3" ;;
    pacman) echo -e "  ${BLUE}sqlite3${RESET}: sudo pacman -S sqlite" ;;
    zypper) echo -e "  ${BLUE}sqlite3${RESET}: sudo zypper install sqlite3" ;;
    apk) echo -e "  ${BLUE}sqlite3${RESET}: sudo apk add sqlite" ;;
    *) echo -e "  ${BLUE}sqlite3${RESET}: Install sqlite3 using your package manager" ;;
    esac
    ;;
  systemd)
    if [[ "$OS" == "macos" ]]; then
      echo -e "  ${BLUE}systemd${RESET}: Not available on macOS (production installs not supported)"
    else
      echo -e "  ${BLUE}systemd${RESET}: Should be available on your Linux distribution"
    fi
    ;;
  esac
}

# Check production availability - sets global variables
check_production_availability() {
  echo -e "${BLUE}Checking production capabilities...${RESET}"
  MISSING_PRODUCTION_DEPS=()

  # Check nginx
  if ! command -v nginx &>/dev/null; then
    MISSING_PRODUCTION_DEPS+=("nginx")
    echo -e "${YELLOW}⚠️  nginx not found${RESET}"
  else
    echo -e "${GREEN}✅ nginx found${RESET}"
  fi

  # Check pm2
  if ! command -v pm2 &>/dev/null; then
    MISSING_PRODUCTION_DEPS+=("pm2")
    echo -e "${YELLOW}⚠️  pm2 not found${RESET}"
  else
    echo -e "${GREEN}✅ pm2 found${RESET}"
  fi

  # Check systemctl (not available on macOS)
  if [[ "$OS" == "macos" ]]; then
    MISSING_PRODUCTION_DEPS+=("systemd")
    echo -e "${YELLOW}⚠️  systemd not available on macOS${RESET}"
  elif ! command -v systemctl &>/dev/null; then
    MISSING_PRODUCTION_DEPS+=("systemd")
    echo -e "${YELLOW}⚠️  systemd not found${RESET}"
  else
    echo -e "${GREEN}✅ systemd found${RESET}"
  fi

  # Check certbot (optional)
  if ! command -v certbot &>/dev/null; then
    echo -e "${YELLOW}⚠️  certbot not found (can be installed during setup)${RESET}"
  else
    echo -e "${GREEN}✅ certbot found${RESET}"
  fi

  # Set production availability flag
  if [[ ${#MISSING_PRODUCTION_DEPS[@]} -eq 0 ]] && [[ "$CURRENT_USER" == "root" ]]; then
    PRODUCTION_AVAILABLE=true
  else
    PRODUCTION_AVAILABLE=false
  fi
}

# Check prerequisites
check_prerequisites() {
  echo -e "${BLUE}Checking prerequisites...${RESET}"

  # Detect OS and package manager first
  detect_os_and_package_manager
  echo -e "${BLUE}Detected OS: ${WHITE}${OS}${RESET}, Package Manager: ${WHITE}${PACKAGE_MANAGER}${RESET}"

  # Set current user
  CURRENT_USER=$(whoami)
  echo -e "${BLUE}Running as user: ${WHITE}${CURRENT_USER}${RESET}"

  # Core dependencies needed for all installs

  # Check Go
  if ! command -v go &>/dev/null; then
    echo -e "${RED}❌ Go is not installed. Please install Go 1.22+ first.${RESET}"
    cleanup_lock
    exit 1
  fi

  GO_VERSION=$(go version | grep -o 'go[0-9]\+\.[0-9]\+' | sed 's/go//')
  echo -e "${GREEN}✅ Go ${GO_VERSION} found${RESET}"

  # Check Node.js/pnpm
  if ! command -v node &>/dev/null; then
    echo -e "${RED}❌ Node.js is not installed. Please install Node.js 18+ first.${RESET}"
    cleanup_lock
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
    cleanup_lock
    exit 1
  fi

  echo -e "${GREEN}✅ Git found${RESET}"

  # For interactive mode, check production capabilities now
  if [[ -z "${INSTALL_TYPE}" ]]; then
    echo
    check_production_availability
  fi

  # For CLI mode with production install, validate now
  if [[ "${INSTALL_TYPE}" != "quick" ]] && [[ -n "${INSTALL_TYPE}" ]]; then
    check_production_prerequisites
  fi

  echo
}

# Check production-specific prerequisites (for CLI mode)
check_production_prerequisites() {
  echo -e "${BLUE}Checking production prerequisites...${RESET}"

  if [[ "$CURRENT_USER" != "root" ]]; then
    echo -e "${RED}❌ Production installations must be run as root${RESET}"
    echo "Please run with: sudo $0 $*"
    cleanup_lock
    exit 1
  fi

  # Check nginx
  if ! command -v nginx &>/dev/null; then
    echo -e "${RED}❌ nginx is not installed.${RESET}"
    show_install_instructions "nginx"
    cleanup_lock
    exit 1
  fi
  echo -e "${GREEN}✅ nginx found${RESET}"

  # Check pm2
  if ! command -v pm2 &>/dev/null; then
    echo -e "${YELLOW}⚠️  pm2 not found, installing globally...${RESET}"
    npm install -g pm2
  fi
  echo -e "${GREEN}✅ pm2 found${RESET}"

  # Check systemctl
  if [[ "$OS" == "macos" ]]; then
    echo -e "${RED}❌ Production installations not supported on macOS${RESET}"
    cleanup_lock
    exit 1
  elif ! command -v systemctl &>/dev/null; then
    echo -e "${RED}❌ systemctl not found.${RESET}"
    show_install_instructions "systemd"
    cleanup_lock
    exit 1
  fi
  echo -e "${GREEN}✅ systemd found${RESET}"

  # Check sqlite3
  if ! command -v sqlite3 &>/dev/null; then
    echo -e "${RED}❌ sqlite3 is not installed.${RESET}"
    show_install_instructions "sqlite3"
    cleanup_lock
    exit 1
  fi
  echo -e "${GREEN}✅ sqlite3 found${RESET}"

  # Check if t8k user exists
  if id "t8k" &>/dev/null; then
    echo -e "${GREEN}✅ User 't8k' exists${RESET}"
  else
    echo -e "${YELLOW}⚠️  User 't8k' will be created during installation${RESET}"
  fi
}

# Interactive install choice - uses pre-computed results
choose_install_type() {
  if [[ "$CURRENT_USER" == "root" ]]; then
    echo -e "${YELLOW}Running as root - quick install disabled${RESET}"
    echo

    if [[ "$PRODUCTION_AVAILABLE" == true ]]; then
      # Show production options only
      echo -e "${BLUE}Choose installation type:${RESET}"
      echo "1) Production single-tenant"
      echo "2) Production multi-tenant"
      echo "3) Dedicated tenant"
      echo
      read -p "Enter choice [1-3]: " choice

      case $choice in
      1)
        INSTALL_TYPE="prod"
        ask_domain
        ;;
      2)
        INSTALL_TYPE="multi"
        ask_domain
        ;;
      3)
        INSTALL_TYPE="dedicated"
        ask_dedicated_info
        ;;
      *)
        echo -e "${RED}Invalid choice. Please enter 1-3.${RESET}"
        choose_install_type
        ;;
      esac
    else
      # Missing production dependencies
      echo -e "${RED}Production installations not available.${RESET}"
      echo -e "Missing dependencies: ${RED}${MISSING_PRODUCTION_DEPS[*]}${RESET}"
      echo
      echo -e "To enable production installations, please install:"
      for dep in "${MISSING_PRODUCTION_DEPS[@]}"; do
        show_install_instructions "$dep"
      done
      echo
      echo "Installation cannot proceed."
      cleanup_lock
      exit 1
    fi
  else
    # Not running as root - only quick install
    echo -e "${BLUE}Choose installation type:${RESET}"
    echo "1) Quick install (development setup)"
    echo
    echo -e "${YELLOW}Production installations require root privileges.${RESET}"
    echo -e "Run with ${BLUE}sudo $0${RESET} to access production options."
    echo
    read -p "Continue with quick install? [Y/n]: " continue_quick
    if [[ "$continue_quick" =~ ^[Nn] ]]; then
      echo "Installation cancelled."
      exit 0
    fi
    INSTALL_TYPE="quick"
  fi
}

# Ask for domain in interactive mode
ask_domain() {
  echo
  read -p "Enter your domain (e.g., tractstack.com): " domain
  if [[ -z "$domain" ]]; then
    echo -e "${RED}Domain is required${RESET}"
    ask_domain
  fi
  DOMAIN="$domain"
}

# Ask for dedicated site info
ask_dedicated_info() {
  echo
  read -p "Enter site ID (3-12 lowercase chars): " site_id
  if [[ -z "$site_id" ]] || [[ ! "$site_id" =~ ^[a-z0-9-]{3,12}$ ]]; then
    echo -e "${RED}Invalid site ID format${RESET}"
    ask_dedicated_info
  fi
  SITE_ID="$site_id"

  read -p "Enter your domain (e.g., atriskmedia.com): " domain
  if [[ -z "$domain" ]]; then
    echo -e "${RED}Domain is required${RESET}"
    ask_dedicated_info
  fi
  DOMAIN="$domain"
}

# CLI argument parsing
parse_cli_args() {
  while [[ $# -gt 0 ]]; do
    case $1 in
    --quick)
      INSTALL_TYPE="quick"
      shift
      ;;
    --prod)
      INSTALL_TYPE="prod"
      shift
      ;;
    --multi)
      INSTALL_TYPE="multi"
      shift
      ;;
    --dedicated)
      INSTALL_TYPE="dedicated"
      if [[ -z "${2:-}" ]] || [[ "$2" =~ ^-- ]]; then
        echo -e "${RED}--dedicated requires site ID${RESET}"
        cleanup_lock
        exit 1
      fi
      SITE_ID="$2"
      shift 2
      ;;
    --domain)
      if [[ -z "${2:-}" ]] || [[ "$2" =~ ^-- ]]; then
        echo -e "${RED}--domain requires a domain name${RESET}"
        cleanup_lock
        exit 1
      fi
      DOMAIN="$2"
      shift 2
      ;;
    --non-interactive)
      NON_INTERACTIVE=true
      shift
      ;;
    --help | -h)
      show_usage
      exit 0
      ;;
    *)
      echo -e "${RED}Unknown option: $1${RESET}"
      show_usage
      cleanup_lock
      exit 1
      ;;
    esac
  done

  # Validate required parameters
  if [[ -z "${INSTALL_TYPE}" ]]; then
    echo -e "${RED}Please specify installation type${RESET}"
    show_usage
    cleanup_lock
    exit 1
  fi

  if [[ "${INSTALL_TYPE}" != "quick" ]] && [[ -z "${DOMAIN}" ]]; then
    echo -e "${RED}Domain is required for production installations${RESET}"
    cleanup_lock
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
  echo "  --quick                    Quick development setup"
  echo "  --prod                     Production single-tenant"
  echo "  --multi                    Production multi-tenant"
  echo "  --dedicated <tenant-id>    Dedicated tenant instance"
  echo "  --domain <domain>          Base domain (required for production)"
  echo "  --custom-domain <domain>   Custom domain for dedicated instances"
  echo "  --non-interactive          Fail if manual verification needed"
  echo "  --help, -h                 Show this help message"
  echo
  echo "Examples:"
  echo "  $0 --quick"
  echo "  $0 --prod --domain=tractstack.com"
  echo "  $0 --multi --domain=tractstack.com"
  echo "  $0 --dedicated mytenant --domain=tractstack.com --custom-domain=example.com"
}

# Check if TractStack is already installed
check_existing_installation() {
  case "${INSTALL_TYPE}" in
  "prod" | "multi")
    if [[ -d "/home/t8k/t8k-go-server" ]]; then
      echo -e "${RED}❌ TractStack already installed at /home/t8k/${RESET}"
      echo "Cannot install main/multi instance over existing installation."
      cleanup_lock
      exit 1
    fi
    ;;
  "dedicated")
    if [[ -d "/home/t8k/sites/${SITE_ID}" ]]; then
      echo -e "${RED}❌ Site '${SITE_ID}' already exists at /home/t8k/sites/${SITE_ID}/${RESET}"
      echo "Cannot install over existing dedicated site."
      cleanup_lock
      exit 1
    fi
    ;;
  esac
}

# Detect Cloudflare secrets
detect_cloudflare_secrets() {
  echo -e "${BLUE}Detecting SSL configuration...${RESET}"

  # Check if t8k secrets exist first
  if [[ -f "/home/t8k/.secrets/certbot/cloudflare.ini" ]]; then
    echo -e "${GREEN}✅ Cloudflare DNS secrets found in t8k account - automated SSL enabled${RESET}"
    return 0
  fi

  # Check if root secrets exist
  if [[ -f "/root/.secrets/certbot/cloudflare.ini" ]]; then
    echo -e "${YELLOW}⚠️ Cloudflare DNS secrets found in root account${RESET}"

    if [[ "${NON_INTERACTIVE}" == true ]]; then
      echo -e "${RED}⛌ Cannot copy credentials in non-interactive mode${RESET}"
      cleanup_lock
      exit 1
    fi

    echo -e "${YELLOW}[SECURITY CONSIDERATION]${RESET}"
    echo "Cloudflare private secrets found in root account."
    read -p "Share securely with t8k account? [y/N]: " share_secrets

    if [[ "$share_secrets" =~ ^[Yy] ]]; then
      sudo -u t8k mkdir -p /home/t8k/.secrets/certbot
      cp /root/.secrets/certbot/cloudflare.ini /home/t8k/.secrets/certbot/
      chown t8k:t8k /home/t8k/.secrets/certbot/cloudflare.ini
      chmod 600 /home/t8k/.secrets/certbot/cloudflare.ini
      echo -e "${GREEN}✅ Cloudflare DNS secrets copied - automated SSL enabled${RESET}"
    else
      echo -e "${YELLOW}⚠️ Manual DNS verification will be used${RESET}"
    fi
  else
    echo -e "${YELLOW}⚠️ No Cloudflare secrets found - manual verification will be used${RESET}"
    if [[ "${NON_INTERACTIVE}" == true ]]; then
      echo -e "${RED}⛌ Cannot proceed with manual verification in non-interactive mode${RESET}"
      cleanup_lock
      exit 1
    fi
  fi
}

# Setup directories
setup_directories() {
  echo -e "${BLUE}Setting up directories...${RESET}"

  case "${INSTALL_TYPE}" in
  "prod" | "multi")
    # /home/t8k/ structure
    create_t8k_user
    sudo -u t8k mkdir -p /home/t8k/{src,t8k-go-server,etc/letsencrypt,lib/letsencrypt,log/letsencrypt,state,bin,scripts}
    sudo -u t8k mkdir -p /home/t8k/etc/pm2
    echo -e "${GREEN}✅ Production directories created at /home/t8k/${RESET}"
    ;;
  "dedicated")
    # /home/t8k/sites/{siteId}/ structure + shared dirs
    create_t8k_user
    sudo -u t8k mkdir -p "/home/t8k/sites/${SITE_ID}"/{src,t8k-go-server,state,bin}
    sudo -u t8k mkdir -p /home/t8k/{etc/letsencrypt,lib/letsencrypt,log/letsencrypt,scripts}
    sudo -u t8k mkdir -p /home/t8k/etc/pm2
    echo -e "${GREEN}✅ Dedicated site directories created at /home/t8k/sites/${SITE_ID}/${RESET}"
    ;;
  *)
    echo -e "${RED}❌ Unknown install type: ${INSTALL_TYPE}${RESET}"
    cleanup_lock
    exit 1
    ;;
  esac
}

# Allocate ports
allocate_ports() {
  echo -e "${BLUE}Allocating ports for production instance...${RESET}"

  local used_go_ports=()
  local used_astro_ports=()

  # Ensure the directory and file exist, owned by t8k
  sudo -u t8k mkdir -p "$(dirname "$PORTS_CONFIG_FILE")"
  if [[ ! -f "$PORTS_CONFIG_FILE" ]]; then
    sudo -u t8k touch "$PORTS_CONFIG_FILE"
    sudo chown t8k:t8k "$PORTS_CONFIG_FILE"
    sudo chmod 644 "$PORTS_CONFIG_FILE"
  fi

  # Read existing allocations from the file
  if [[ -f "$PORTS_CONFIG_FILE" ]]; then
    while IFS="=" read -r site_id_entry port_pair; do
      if [[ "$site_id_entry" =~ ^[a-zA-Z0-9_-]+$ ]] && [[ "$port_pair" =~ ^[0-9]+,[0-9]+$ ]]; then
        local go_port=$(echo "$port_pair" | cut -d',' -f1)
        local astro_port=$(echo "$port_pair" | cut -d',' -f2)
        used_go_ports+=("$go_port")
        used_astro_ports+=("$astro_port")
      fi
    done <"$PORTS_CONFIG_FILE"
  fi

  # Determine the current site_id for this installation
  local current_site_id
  if [[ "${INSTALL_TYPE}" == "dedicated" ]]; then
    current_site_id="$SITE_ID"
  elif [[ "${INSTALL_TYPE}" == "prod" || "${INSTALL_TYPE}" == "multi" ]]; then
    current_site_id="main"
  else
    echo -e "${RED}❌ Internal error: allocate_ports called for unsupported install type: ${INSTALL_TYPE}${RESET}"
    cleanup_lock
    exit 1
  fi

  # Check if ports are already allocated for this specific site_id
  local existing_go_port=""
  local existing_astro_port=""
  if [[ -f "$PORTS_CONFIG_FILE" ]]; then
    while IFS="=" read -r site_entry port_val_pair; do
      if [[ "$site_entry" == "$current_site_id" ]]; then
        existing_go_port=$(echo "$port_val_pair" | cut -d',' -f1)
        existing_astro_port=$(echo "$port_val_pair" | cut -d',' -f2)
        break
      fi
    done <"$PORTS_CONFIG_FILE"
  fi

  if [[ -n "$existing_go_port" ]] && [[ -n "$existing_astro_port" ]]; then
    ALLOCATED_GO_PORT="$existing_go_port"
    ALLOCATED_ASTRO_PORT="$existing_astro_port"
    echo -e "${GREEN}✅ Re-using existing allocated ports for ${current_site_id}: Go Port ${ALLOCATED_GO_PORT}, Astro Port ${ALLOCATED_ASTRO_PORT}${RESET}"
    return 0
  fi

  # Find the next available Go port
  local go_port_start=10000
  if [[ "${INSTALL_TYPE}" == "dedicated" ]]; then
    go_port_start=10001
  fi

  ALLOCATED_GO_PORT=""
  for ((port = go_port_start; ; port++)); do
    local is_used=false
    for used_port in "${used_go_ports[@]}"; do
      if [[ "$port" -eq "$used_port" ]]; then
        is_used=true
        break
      fi
    done
    if [[ "$is_used" == false ]]; then
      ALLOCATED_GO_PORT="$port"
      break
    fi
  done

  # Find the next available Astro port
  local astro_port_start=20000
  if [[ "${INSTALL_TYPE}" == "dedicated" ]]; then
    astro_port_start=20001
  fi

  ALLOCATED_ASTRO_PORT=""
  for ((port = astro_port_start; ; port++)); do
    local is_used=false
    for used_port in "${used_astro_ports[@]}"; do
      if [[ "$port" -eq "$used_port" ]]; then
        is_used=true
        break
      fi
    done
    if [[ "$is_used" == false ]]; then
      ALLOCATED_ASTRO_PORT="$port"
      break
    fi
  done

  if [[ -z "$ALLOCATED_GO_PORT" ]] || [[ -z "$ALLOCATED_ASTRO_PORT" ]]; then
    echo -e "${RED}❌ Failed to allocate Go or Astro ports.${RESET}"
    cleanup_lock
    exit 1
  fi

  echo -e "${GREEN}✅ Allocated new ports for ${current_site_id}: Go Port ${ALLOCATED_GO_PORT}, Astro Port ${ALLOCATED_ASTRO_PORT}${RESET}"

  local new_entry="${current_site_id}=${ALLOCATED_GO_PORT},${ALLOCATED_ASTRO_PORT}"
  local temp_ports_file="${PORTS_CONFIG_FILE}.tmp"
  local entry_found=false

  >"$temp_ports_file"
  if [[ -f "$PORTS_CONFIG_FILE" ]]; then
    while IFS= read -r line; do
      if [[ "$line" == "${current_site_id}"=* ]]; then
        echo "$new_entry" >>"$temp_ports_file"
        entry_found=true
      else
        echo "$line" >>"$temp_ports_file"
      fi
    done <"$PORTS_CONFIG_FILE"
  fi

  if [[ "$entry_found" == false ]]; then
    echo "$new_entry" >>"$temp_ports_file"
  fi

  sudo mv "$temp_ports_file" "$PORTS_CONFIG_FILE"
  sudo chown t8k:t8k "$PORTS_CONFIG_FILE"
  sudo chmod 644 "$PORTS_CONFIG_FILE"
  echo -e "${GREEN}✅ Ports configuration updated in ${PORTS_CONFIG_FILE}${RESET}"
}

# Deploy Go backend
deploy_go_backend() {
  echo -e "${BLUE}Deploying Go backend...${RESET}"

  local base_dir
  if [[ "${INSTALL_TYPE}" == "dedicated" ]]; then
    base_dir="/home/t8k/sites/${SITE_ID}"
  else
    base_dir="/home/t8k"
  fi

  local src_dir="${base_dir}/src"
  local bin_dir="${base_dir}/bin"
  local data_dir="${base_dir}/t8k-go-server"

  echo -e "${BLUE}Cloning TractStack Go backend...${RESET}"
  sudo -u t8k git clone https://github.com/AtRiskMedia/tractstack-go.git "${src_dir}/tractstack-go"

  echo -e "${BLUE}Creating Go backend configuration...${RESET}"
  local go_env_file="${src_dir}/tractstack-go/.env"
  sudo -u t8k tee "$go_env_file" >/dev/null <<EOF
GO_BACKEND_PATH=${data_dir}/
PORT=${ALLOCATED_GO_PORT}
GIN_MODE=release
EOF

  if [[ "${INSTALL_TYPE}" == "multi" ]]; then
    echo "ENABLE_MULTI_TENANT=true" | sudo -u t8k tee -a "$go_env_file" >/dev/null
  fi

  echo -e "${BLUE}Building Go backend...${RESET}"
  cd "${src_dir}/tractstack-go"
  sudo -u t8k go build -o "${bin_dir}/tractstack-go" ./cmd/tractstack-go

  # Copy operational scripts from the repo to the central scripts directory
  if [[ -d "${src_dir}/tractstack-go/pkg/scripts" ]]; then
    echo -e "${BLUE}Deploying operational scripts...${RESET}"
    sudo -u t8k cp -r "${src_dir}/tractstack-go/pkg/scripts/"* /home/t8k/scripts/
    sudo -u t8k chmod +x /home/t8k/scripts/*
  fi

  echo -e "${GREEN}✅ Go backend deployed to ${bin_dir}/tractstack-go${RESET}"
}

# Deploy Astro frontend
deploy_astro_frontend() {
  echo -e "${BLUE}Deploying Astro frontend...${RESET}"

  local base_dir
  local tenant_id
  tenant_id="default"
  if [[ "${INSTALL_TYPE}" == "dedicated" ]]; then
    base_dir="/home/t8k/sites/${SITE_ID}"
  else
    base_dir="/home/t8k"
  fi

  local src_dir="${base_dir}/src"
  local data_dir="${base_dir}/t8k-go-server"

  echo -e "${BLUE}Creating Astro frontend project...${RESET}"
  cd "$src_dir"
  sudo -u t8k pnpm create astro@latest my-tractstack --template minimal --typescript strict --install

  echo -e "${BLUE}Installing TractStack integration...${RESET}"
  cd "${src_dir}/my-tractstack"
  sudo -u t8k pnpm add astro-tractstack@latest

  echo -e "${BLUE}Creating Astro frontend configuration...${RESET}"
  local astro_env_file="${src_dir}/my-tractstack/.env"
  sudo -u t8k tee "$astro_env_file" >/dev/null <<EOF
PRIVATE_GO_BACKEND_PATH=${data_dir}/
PUBLIC_GO_BACKEND=http://localhost:${ALLOCATED_GO_PORT}
PUBLIC_TENANTID=${tenant_id}
EOF

  if [[ "${INSTALL_TYPE}" == "multi" ]]; then
    echo "ENABLE_MULTI_TENANT=true" | sudo -u t8k tee -a "$astro_env_file" >/dev/null
  fi

  echo -e "${BLUE}Running TractStack setup...${RESET}"
  sudo -u t8k npx create-tractstack

  echo -e "${BLUE}Building Astro frontend for production...${RESET}"
  sudo -u t8k pnpm build

  if [[ ! -d "${src_dir}/my-tractstack/dist" ]]; then
    echo -e "${RED}❌ Astro build failed - no dist directory found${RESET}"
    cleanup_lock
    exit 1
  fi

  echo -e "${GREEN}✅ Astro frontend deployed and built at ${src_dir}/my-tractstack${RESET}"
}

# Prepare service artifacts
prepare_service_artifacts() {
  echo -e "${BLUE}Preparing service artifacts...${RESET}"

  local base_dir
  if [[ "${INSTALL_TYPE}" == "dedicated" ]]; then
    base_dir="/home/t8k/sites/${SITE_ID}"
  else
    base_dir="/home/t8k"
  fi

  local src_dir="${base_dir}/src"
  local bin_dir="${base_dir}/bin"

  local go_binary="${bin_dir}/tractstack-go"
  if [[ ! -f "$go_binary" ]]; then
    echo -e "${RED}❌ Go binary not found at ${go_binary}${RESET}"
    cleanup_lock
    exit 1
  fi
  if [[ ! -x "$go_binary" ]]; then
    echo -e "${BLUE}Making Go binary executable...${RESET}"
    sudo -u t8k chmod +x "$go_binary"
  fi

  local astro_dist="${src_dir}/my-tractstack/dist"
  if [[ ! -d "$astro_dist" ]]; then
    echo -e "${RED}❌ Astro build artifacts not found at ${astro_dist}${RESET}"
    cleanup_lock
    exit 1
  fi

  local package_json="${src_dir}/my-tractstack/package.json"
  if [[ ! -f "$package_json" ]]; then
    echo -e "${RED}❌ package.json not found at ${package_json}${RESET}"
    cleanup_lock
    exit 1
  fi

  local state_dir
  if [[ "${INSTALL_TYPE}" == "dedicated" ]]; then
    state_dir="${base_dir}/state"
  else
    state_dir="/home/t8k/state"
  fi

  sudo -u t8k mkdir -p "$state_dir"
  sudo -u t8k chmod 755 "$state_dir"

  local go_env_file="${src_dir}/tractstack-go/.env"
  if [[ ! -f "$go_env_file" ]]; then
    echo -e "${RED}❌ Go backend .env file not found${RESET}"
    cleanup_lock
    exit 1
  fi

  local astro_env_file="${src_dir}/my-tractstack/.env"
  if [[ ! -f "$astro_env_file" ]]; then
    echo -e "${RED}❌ Astro .env file not found${RESET}"
    cleanup_lock
    exit 1
  fi

  echo -e "${GREEN}✅ Service artifacts prepared and validated${RESET}"
}

# Create t8k user if needed
create_t8k_user() {
  echo -e "${BLUE}Setting up t8k user...${RESET}"

  if id "t8k" &>/dev/null; then
    echo -e "${GREEN}✅ User 't8k' already exists${RESET}"
    return 0
  fi

  if [[ "${NON_INTERACTIVE}" == true ]]; then
    echo -e "${RED}❌ User 't8k' does not exist and cannot create in non-interactive mode${RESET}"
    cleanup_lock
    exit 1
  fi

  echo -e "${BLUE}Creating user 't8k'...${RESET}"

  case $PACKAGE_MANAGER in
  apt)
    adduser t8k
    ;;
  dnf | pacman | zypper | apk)
    useradd -m t8k
    echo "Please set password for user 't8k':"
    passwd t8k
    ;;
  *)
    useradd -m t8k
    echo "Please set password for user 't8k':"
    passwd t8k
    ;;
  esac

  echo -e "${GREEN}✅ User 't8k' created successfully${RESET}"
}

# Setup SSL certificates
setup_ssl_certificates() {
  if [[ "$DRY_RUN" == "true" ]]; then
    echo -e "${BLUE}Setting up SSL certificates...${RESET} [SIMULATION]"
    dry_run_flag="--dry-run"
  else
    echo -e "${BLUE}Setting up SSL certificates...${RESET}"
    dry_run_flag=""
  fi

  if ! command -v certbot &>/dev/null; then
    echo -e "${BLUE}Installing certbot in t8k user venv...${RESET}"
    sudo -u t8k bash -c "
      python3 -m venv /home/t8k/certbot_venv
      source /home/t8k/certbot_venv/bin/activate
      pip install certbot certbot-dns-cloudflare
    "
    echo -e "${GREEN}✅ Certbot installed in /home/t8k/certbot_venv${RESET}"
  fi

  sudo -u t8k mkdir -p /home/t8k/etc/letsencrypt /home/t8k/lib/letsencrypt /home/t8k/log/letsencrypt

  local primary_cert_domains="-d ${DOMAIN} -d *.${DOMAIN}"
  local primary_cert_path="/home/t8k/etc/letsencrypt/live/${DOMAIN}"
  local cert_file="${primary_cert_path}/fullchain.pem"
  local key_file="${primary_cert_path}/privkey.pem"

  if [[ -f "$cert_file" ]] && [[ -f "$key_file" ]]; then
    echo -e "${BLUE}Checking existing certificate validity...${RESET}"
    if openssl x509 -in "$cert_file" -noout -checkend 2592000 >/dev/null 2>&1; then
      echo -e "${GREEN}✅ Valid SSL certificates already exist (not expiring within 30 days)${RESET}"
      return 0
    else
      echo -e "${YELLOW}⚠️ Existing certificates expire within 30 days - will renew${RESET}"
    fi
  else
    echo -e "${BLUE}No existing certificates found at ${primary_cert_path}${RESET}"
  fi

  echo -e "${BLUE}Requesting SSL certificates for: ${primary_cert_domains}${RESET}"

  if [[ -f "/home/t8k/.secrets/certbot/cloudflare.ini" ]]; then
    echo -e "${GREEN}Using Cloudflare DNS automation${RESET}"
    sudo -u t8k bash -c "
      source /home/t8k/certbot_venv/bin/activate
      certbot certonly --dns-cloudflare \
        --dns-cloudflare-credentials /home/t8k/.secrets/certbot/cloudflare.ini \
        --dns-cloudflare-propagation-seconds 15 \
        --config-dir /home/t8k/etc/letsencrypt \
        --work-dir /home/t8k/lib/letsencrypt \
        --logs-dir /home/t8k/log/letsencrypt \
        --non-interactive --agree-tos \
        --email admin@${DOMAIN} \
        $dry_run_flag \
        ${primary_cert_domains}
    "
  else
    echo -e "${YELLOW}Using manual DNS verification${RESET}"
    echo -e "${BLUE}Certbot will show you TXT records to add to your DNS...${RESET}"
    if [[ "${NON_INTERACTIVE}" == true ]]; then
      echo -e "${RED}⛌ Manual verification required but running in non-interactive mode${RESET}"
      cleanup_lock
      exit 1
    fi
    sudo -u t8k bash -c "
      source /home/t8k/certbot_venv/bin/activate
      certbot certonly --manual --preferred-challenges dns \
        --config-dir /home/t8k/etc/letsencrypt \
        --work-dir /home/t8k/lib/letsencrypt \
        --logs-dir /home/t8k/log/letsencrypt \
        --agree-tos --email admin@${DOMAIN} \
        $dry_run_flag \
        ${primary_cert_domains}
    "
  fi
  echo -e "${GREEN}✅ SSL certificate request completed${RESET}"
  echo -e "${WHITE}SUCCESS! Visit ${BLUE}https://${DOMAIN}${RESET} or ${BLUE}https://localhost:${ALLOCATED_ASTRO_PORT}${RESET}"
}

# Configure nginx
configure_nginx() {
  echo -e "${BLUE}Configuring nginx...${RESET}"

  if ! systemctl is-active --quiet nginx; then
    systemctl start nginx
    systemctl enable nginx
  fi

  local config_name
  if [[ "${INSTALL_TYPE}" == "dedicated" ]]; then
    config_name="t8k-${SITE_ID}"
  else
    config_name="t8k-default"
  fi

  mkdir -p /etc/nginx/sites-available /etc/nginx/sites-enabled
  local config_file="/etc/nginx/sites-available/${config_name}.conf"

  case "${INSTALL_TYPE}" in
  "prod")
    cat >"$config_file" <<EOF
# HTTP to HTTPS redirect
server {
  listen 80;
  server_name ${DOMAIN} www.${DOMAIN};
  return 301 https://\$server_name\$request_uri;
}
# HTTPS server
server {
  listen 443 ssl http2;
  server_name ${DOMAIN} www.${DOMAIN};
  ssl_certificate /home/t8k/etc/letsencrypt/live/${DOMAIN}/fullchain.pem;
  ssl_certificate_key /home/t8k/etc/letsencrypt/live/${DOMAIN}/privkey.pem;
  access_log off;
  error_log /var/log/nginx/${config_name}-error.log warn;
  location / {
      proxy_pass http://localhost:${ALLOCATED_ASTRO_PORT};
      proxy_set_header Host \$host;
      proxy_set_header X-Real-IP \$remote_addr;
      proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto \$scheme;
  }
  location /media/ {
      alias /home/t8k/t8k-go-server/config/default/media/;
  }
}
EOF
    ;;
  "multi")
    cat >"$config_file" <<EOF
# HTTP to HTTPS redirect
server {
  listen 80;
  server_name ${DOMAIN} www.${DOMAIN} *.${DOMAIN};
  return 301 https://\$server_name\$request_uri;
}
# HTTPS server
server {
  listen 443 ssl http2;
  server_name ${DOMAIN} www.${DOMAIN} *.${DOMAIN};
  ssl_certificate /home/t8k/etc/letsencrypt/live/${DOMAIN}/fullchain.pem;
  ssl_certificate_key /home/t8k/etc/letsencrypt/live/${DOMAIN}/privkey.pem;
  access_log off;
  error_log /var/log/nginx/${config_name}-error.log warn;
  location / {
      proxy_pass http://localhost:${ALLOCATED_ASTRO_PORT};
      proxy_set_header Host \$host;
      proxy_set_header X-Real-IP \$remote_addr;
      proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto \$scheme;
  }
  location /media/ {
      set \$tenant_dir "default";
      if (\$host ~* ^([^.]+)\\.${DOMAIN//./\\.}\$) {
          set \$tenant_dir \$1;
      }
      alias /home/t8k/t8k-go-server/config/\$tenant_dir/media/;
  }
}
EOF
    ;;
  "dedicated")
    cat >"$config_file" <<EOF
# HTTP to HTTPS redirect
server {
  listen 80;
  server_name ${DOMAIN} www.${DOMAIN};
  return 301 https://\$server_name\$request_uri;
}
# HTTPS server
server {
  listen 443 ssl http2;
  server_name ${DOMAIN} www.${DOMAIN};
  ssl_certificate /home/t8k/etc/letsencrypt/live/${DOMAIN}/fullchain.pem;
  ssl_certificate_key /home/t8k/etc/letsencrypt/live/${DOMAIN}/privkey.pem;
  access_log off;
  error_log /var/log/nginx/${config_name}-error.log warn;
  location / {
      proxy_pass http://localhost:${ALLOCATED_ASTRO_PORT};
      proxy_set_header Host \$host;
      proxy_set_header X-Real-IP \$remote_addr;
      proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto \$scheme;
  }
  location /media/ {
      alias /home/t8k/sites/${SITE_ID}/t8k-go-server/config/${SITE_ID}/media/;
  }
}
EOF
    ;;
  *)
    echo -e "${RED}⛌ Unknown install type: ${INSTALL_TYPE}${RESET}"
    cleanup_lock
    exit 1
    ;;
  esac

  echo -e "${BLUE}Creating nginx site configuration: ${config_name}.conf${RESET}"
  ln -sf "$config_file" "/etc/nginx/sites-enabled/${config_name}.conf"
  if ! nginx -t; then
    echo -e "${RED}⛌ nginx configuration test failed${RESET}"
    rm -f "/etc/nginx/sites-enabled/${config_name}.conf"
    cleanup_lock
    exit 1
  fi
  systemctl reload nginx
  echo -e "${GREEN}✅ nginx configuration complete${RESET}"
}

# Configure systemd services
configure_systemd_services() {
  echo -e "${BLUE}Configuring systemd services...${RESET}"

  local service_name
  local service_file
  local binary_path
  local working_dir
  local data_dir

  case "${INSTALL_TYPE}" in
  "prod" | "multi")
    service_name="tractstack-go"
    service_file="/etc/systemd/system/tractstack-go.service"
    binary_path="/home/t8k/bin/tractstack-go"
    working_dir="/home/t8k/src/tractstack-go"
    data_dir="/home/t8k/t8k-go-server"
    ;;
  "dedicated")
    service_name="tractstack-go@${SITE_ID}"
    service_file="/etc/systemd/system/tractstack-go@.service"
    binary_path="/home/t8k/sites/%i/bin/tractstack-go"
    working_dir="/home/t8k/sites/%i/src/tractstack-go"
    data_dir="/home/t8k/sites/%i/t8k-go-server"
    ;;
  *)
    echo -e "${RED}⛌ Unknown install type: ${INSTALL_TYPE}${RESET}"
    cleanup_lock
    exit 1
    ;;
  esac

  echo -e "${BLUE}Creating systemd service: ${service_name}${RESET}"

  if [[ "${INSTALL_TYPE}" == "dedicated" ]]; then
    cat >"$service_file" <<EOF
[Unit]
Description=TractStack Go Backend (Site: %i)
After=network-online.target nginx.service
Wants=network-online.target
Requires=nginx.service
[Service]
Type=simple
User=t8k
Group=t8k
WorkingDirectory=${working_dir}
Environment=GO_BACKEND_PATH=${data_dir}/
Environment=PORT=${ALLOCATED_GO_PORT}
Environment=GIN_MODE=release
ExecStart=${binary_path}
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=tractstack-go-%i
NoNewPrivileges=yes
PrivateTmp=yes
ReadWritePaths=/home/t8k
[Install]
WantedBy=multi-user.target
EOF
  else
    cat >"$service_file" <<EOF
[Unit]
Description=TractStack Go Backend
After=network-online.target nginx.service
Wants=network-online.target
Requires=nginx.service
[Service]
Type=simple
User=t8k
Group=t8k
WorkingDirectory=${working_dir}
Environment=GO_BACKEND_PATH=${data_dir}/
Environment=PORT=${ALLOCATED_GO_PORT}
Environment=GIN_MODE=release$(if [[ "${INSTALL_TYPE}" == "multi" ]]; then echo -e "\nEnvironment=ENABLE_MULTI_TENANT=true"; fi)
ExecStart=${binary_path}
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=tractstack-go
NoNewPrivileges=yes
PrivateTmp=yes
ReadWritePaths=/home/t8k
[Install]
WantedBy=multi-user.target
EOF
  fi
  echo -e "${GREEN}✅ Created systemd service file: ${service_file}${RESET}"

  setup_build_watcher

  echo -e "${BLUE}Reloading systemd daemon...${RESET}"
  systemctl daemon-reload

  echo -e "${BLUE}Enabling and starting ${service_name}...${RESET}"
  systemctl enable "$service_name"
  systemctl start "$service_name"

  if systemctl is-active --quiet "$service_name"; then
    echo -e "${GREEN}✅ ${service_name} is running${RESET}"
  else
    echo -e "${RED}⛌ ${service_name} failed to start${RESET}"
    echo "Service status:"
    systemctl status "$service_name" --no-pager -l
    cleanup_lock
    exit 1
  fi

  echo -e "${GREEN}✅ systemd service configuration complete${RESET}"
}

# Setup build system file watcher
setup_build_watcher() {
  local path_unit="/etc/systemd/system/t8k-build-watcher.path"
  local service_unit="/etc/systemd/system/t8k-build-watcher.service"

  if systemctl list-unit-files | grep -q "t8k-build-watcher.path"; then
    echo -e "${BLUE}Build system watcher already configured, skipping...${RESET}"
    return 0
  fi

  echo -e "${BLUE}Setting up build system file watcher...${RESET}"

  cat >"$path_unit" <<EOF
[Unit]
Description=TractStack Build Watcher
[Path]
PathModified=/home/t8k/state
Unit=t8k-build-watcher.service
[Install]
WantedBy=multi-user.target
EOF

  cat >"$service_unit" <<EOF
[Unit]
Description=TractStack Build Concierge
[Service]
Type=oneshot
User=t8k
Group=t8k
ExecStart=/home/t8k/scripts/t8k-concierge.sh
StandardOutput=journal
StandardError=journal
SyslogIdentifier=t8k-concierge
[Install]
WantedBy=multi-user.target
EOF
  echo -e "${GREEN}✅ Created build watcher systemd units${RESET}"

  echo -e "${BLUE}Enabling build system file watcher...${RESET}"
  systemctl enable t8k-build-watcher.path
  systemctl start t8k-build-watcher.path

  if systemctl is-active --quiet t8k-build-watcher.path; then
    echo -e "${GREEN}✅ Build system file watcher is active${RESET}"
  else
    echo -e "${YELLOW}⚠️  Build system file watcher failed to start${RESET}"
    systemctl status t8k-build-watcher.path --no-pager -l
  fi
}

# Setup PM2 ecosystem
setup_pm2_ecosystem() {
  echo -e "${BLUE}Setting up PM2 ecosystem...${RESET}"

  local ecosystem_file="/home/t8k/etc/pm2/ecosystem.config.js"
  local pm2_home="/home/t8k/.pm2"

  sudo -u t8k mkdir -p "$pm2_home"

  if [[ ! -f "$ecosystem_file" ]]; then
    echo -e "${BLUE}Creating PM2 ecosystem configuration...${RESET}"
    sudo -u t8k tee "$ecosystem_file" >/dev/null <<'EOF'
const fs = require('fs');
const path = require('path');

const portsFile = '/home/t8k/etc/t8k-ports.conf';
const apps = [];

if (fs.existsSync(portsFile)) {
  const portsContent = fs.readFileSync(portsFile, 'utf8');
  const lines = portsContent.trim().split('\n');

  for (const line of lines) {
    if (!line.includes('=')) continue;

    const [siteId, ports] = line.split('=');
    const [goPort, astroPort] = ports.split(',');

    let appPath;
    let appName;

    if (siteId === 'main') {
      appPath = '/home/t8k/src/my-tractstack';
      appName = 'astro-main';
    } else {
      appPath = `/home/t8k/sites/${siteId}/src/my-tractstack`;
      appName = `astro-${siteId}`;
    }

    if (fs.existsSync(appPath) && fs.existsSync(path.join(appPath, 'dist/server/entry.mjs'))) {
      apps.push({
        name: appName,
        script: 'dist/server/entry.mjs',
        cwd: appPath,
        instances: 1,
        autorestart: true,
        watch: false,
        max_memory_restart: '512M',
        env: {
          NODE_ENV: 'production',
          PORT: astroPort,
          HOST: '0.0.0.0'
        }
      });
    }
  }
}

module.exports = {
  apps: apps
};
EOF
  else
    echo -e "${BLUE}PM2 ecosystem configuration already exists, skipping creation${RESET}"
  fi

  sudo -u t8k mkdir -p /home/t8k/log

  echo -e "${BLUE}Starting or reloading PM2 ecosystem...${RESET}"
  sudo -u t8k --preserve-env=PATH PM2_HOME="$pm2_home" pm2 startOrReload "$ecosystem_file"

  echo -e "${BLUE}Configuring PM2 startup service...${RESET}"
  local pm2_path
  pm2_path=$(sudo -u t8k which pm2)
  if [[ -z "$pm2_path" ]]; then
    echo -e "${RED}❌ Could not find pm2 executable for user t8k${RESET}"
    cleanup_lock
    exit 1
  fi
  env PATH=$PATH:/usr/bin:"$(dirname "$pm2_path")" "$pm2_path" startup systemd -u t8k --hp /home/t8k

  echo -e "${BLUE}Saving process list for reboot...${RESET}"
  sudo -u t8k PM2_HOME="$pm2_home" pm2 save

  echo -e "${GREEN}✅ PM2 ecosystem configured and running${RESET}"
}

# Quick install implementation
quick_install() {
  USER=$(whoami)
  INSTALL_DIR="/home/$USER/t8k"

  echo -e "${BLUE}Starting TractStack quick installation...${RESET}"
  echo -e "Install directory: ${WHITE}$INSTALL_DIR${RESET}"
  echo

  if [[ -d "$INSTALL_DIR" ]]; then
    echo -e "${RED}❌ Directory $INSTALL_DIR already exists!${RESET}"
    echo "For a fresh installation you must remove this folder."
    cleanup_lock
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

# Production install orchestration
production_install() {
  echo -e "${BLUE}Starting TractStack production installation...${RESET}"
  echo -e "Type: ${WHITE}${INSTALL_TYPE}${RESET}"
  echo -e "Domain: ${WHITE}${DOMAIN}${RESET}"
  if [[ "${INSTALL_TYPE}" == "dedicated" ]]; then
    echo -e "Site ID: ${WHITE}${SITE_ID}${RESET}"
  fi
  echo

  check_existing_installation
  setup_directories
  detect_cloudflare_secrets
  setup_ssl_certificates
  allocate_ports
  deploy_go_backend
  deploy_astro_frontend
  prepare_service_artifacts
  configure_nginx
  configure_systemd_services
  setup_pm2_ecosystem

  echo
  echo -e "${GREEN}🎉 TractStack production installation complete!${RESET}"
  echo
}

# Main execution
main() {
  show_header

  if is_interactive "$@"; then
    check_prerequisites
    choose_install_type
    if [[ "${INSTALL_TYPE}" == "quick" ]]; then
      quick_install
    else
      production_install
    fi
  else
    parse_cli_args "$@"
    check_prerequisites
    if [[ "${INSTALL_TYPE}" == "quick" ]]; then
      quick_install
    else
      production_install
    fi
  fi
  cleanup_lock
}

# Run main function with all arguments
main "$@"
exit 0
