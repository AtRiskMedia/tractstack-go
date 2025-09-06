#!/bin/bash

# TractStack v2 Installation Script
# Final Architecture: Configure t8k user environment, then use it via login shells.
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
ALLOCATED_GO_PORT=""
ALLOCATED_ASTRO_PORT=""
PORTS_CONFIG_FILE="/home/t8k/etc/t8k-ports.conf"

cleanup_lock() {
  if [[ -n "${LOCK_FILE:-}" ]]; then
    if [[ -f "$LOCK_FILE" ]]; then
      rm -f "$LOCK_FILE"
    fi
  fi
}

# Check for installation lock
LOCK_FILE="/tmp/t8k-install.lock"
if [[ -f "$LOCK_FILE" ]]; then
  echo -e "${RED}❌ Another TractStack installation is already running${RESET}"
  echo "Lock file: $LOCK_FILE"
  exit 1
fi
# The lock file is touched by the user
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
  echo -e "${YELLOW}To install ${dep}, you can try:${RESET}"
  case $dep in
  nginx)
    case $PACKAGE_MANAGER in
    apt) echo -e "  ${BLUE}sudo apt update && sudo apt install nginx${RESET}" ;;
    dnf) echo -e "  ${BLUE}sudo dnf install nginx${RESET}" ;;
    pacman) echo -e "  ${BLUE}sudo pacman -S nginx${RESET}" ;;
    zypper) echo -e "  ${BLUE}sudo zypper install nginx${RESET}" ;;
    apk) echo -e "  ${BLUE}sudo apk add nginx${RESET}" ;;
    brew) echo -e "  ${BLUE}brew install nginx${RESET}" ;;
    *) echo -e "  ${YELLOW}Please install nginx using your system's package manager.${RESET}" ;;
    esac
    ;;
  pm2)
    echo -e "  ${BLUE}sudo npm install -g pm2${RESET}"
    ;;
  pnpm)
    echo -e "  ${BLUE}sudo npm install -g pnpm${RESET}"
    ;;
  sqlite3)
    case $PACKAGE_MANAGER in
    apt) echo -e "  ${BLUE}sudo apt update && sudo apt install sqlite3${RESET}" ;;
    dnf) echo -e "  ${BLUE}sudo dnf install sqlite3${RESET}" ;;
    pacman) echo -e "  ${BLUE}sudo pacman -S sqlite${RESET}" ;;
    zypper) echo -e "  ${BLUE}sudo zypper install sqlite3${RESET}" ;;
    apk) echo -e "  ${BLUE}sudo apk add sqlite${RESET}" ;;
    *) echo -e "  ${YELLOW}Please install sqlite3 using your system's package manager.${RESET}" ;;
    esac
    ;;
  systemd)
    if [[ "$OS" == "macos" ]]; then
      echo -e "  ${YELLOW}systemd is not available on macOS. Production installs are not supported.${RESET}"
    else
      echo -e "  ${YELLOW}systemd should be included with your Linux distribution.${RESET}"
    fi
    ;;
  python3)
    case $PACKAGE_MANAGER in
    apt) echo -e "  ${BLUE}sudo apt update && sudo apt install python3${RESET}" ;;
    dnf) echo -e "  ${BLUE}sudo dnf install python3${RESET}" ;;
    pacman) echo -e "  ${BLUE}sudo pacman -S python${RESET}" ;;
    zypper) echo -e "  ${BLUE}sudo zypper install python3${RESET}" ;;
    apk) echo -e "  ${BLUE}sudo apk add python3${RESET}" ;;
    brew) echo -e "  ${BLUE}brew install python@3.12${RESET}" ;;
    *) echo -e "  ${YELLOW}Please install python3 using your system's package manager.${RESET}" ;;
    esac
    ;;
  python3-bs4)
    case $PACKAGE_MANAGER in
    apt) echo -e "  ${BLUE}sudo apt update && sudo apt install python3-bs4${RESET}" ;;
    dnf) echo -e "  ${BLUE}sudo dnf install python3-beautifulsoup4${RESET}" ;;
    pacman) echo -e "  ${BLUE}sudo pacman -S python-beautifulsoup4${RESET}" ;;
    zypper) echo -e "  ${BLUE}sudo zypper install python3-beautifulsoup4${RESET}" ;;
    apk) echo -e "  ${BLUE}sudo apk add py3-beautifulsoup4${RESET}" ;;
    brew) echo -e "  ${BLUE}pip3 install beautifulsoup4${RESET}" ;;
    *) echo -e "  ${YELLOW}Please install python3-beautifulsoup4 using your system's package manager.${RESET}" ;;
    esac
    ;;
  esac
}

# Check if user has sudo privileges
has_sudo() {
  if sudo -v 2>/dev/null; then
    echo -e "${YELLOW}Sudo access has been confirmed.${RESET}"
    return 0
  else
    return 1
  fi
}

# Show header conditionally
show_header() {
  # Only show header for interactive runs without CLI args
  if [[ $# -eq 0 ]] && [[ "${NON_INTERACTIVE}" != true ]]; then
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
  fi
}

# Ask for domain in interactive mode
ask_domain() {
  echo
  read -p "Enter your domain (e.g., tractstack.com): " domain </dev/tty
  if [[ -z "$domain" ]]; then
    echo -e "${RED}Domain is required${RESET}"
    ask_domain
  fi
  DOMAIN="$domain"
}

# Ask for dedicated site info
ask_dedicated_info() {
  echo
  read -p "Enter site ID (3-12 lowercase chars): " site_id </dev/tty
  if [[ -z "$site_id" ]] || [[ ! "$site_id" =~ ^[a-z0-9-]{3,12}$ ]]; then
    echo -e "${RED}Invalid site ID format${RESET}"
    ask_dedicated_info
  fi
  SITE_ID="$site_id"

  read -p "Enter your domain (e.g., atriskmedia.com): " domain </dev/tty
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

  # Check if t8k already has secrets first
  if sudo test -f "/home/t8k/.secrets/certbot/cloudflare.ini"; then
    echo "✅ Cloudflare DNS secrets already configured for t8k user"
    return 0
  fi

  # This check requires sudo because it's looking in /root
  if sudo test -f "/root/.secrets/certbot/cloudflare.ini"; then
    echo -e "${YELLOW}⚠️ Cloudflare DNS secrets found in root account${RESET}"

    if [[ "${NON_INTERACTIVE}" == true ]]; then
      echo -e "${RED}⛌ Cannot copy credentials in non-interactive mode${RESET}"
      cleanup_lock
      exit 1
    fi

    echo -e "${YELLOW}[SECURITY CONSIDERATION]${RESET}"
    echo "Cloudflare private secrets found in root account."
    read -p "Share securely with t8k account? [y/N]: " share_secrets </dev/tty

    if [[ "$share_secrets" =~ ^[Yy] ]]; then
      sudo -u t8k mkdir -p /home/t8k/.secrets/certbot
      sudo cp /root/.secrets/certbot/cloudflare.ini /home/t8k/.secrets/certbot/
      sudo chown t8k:t8k /home/t8k/.secrets/certbot/cloudflare.ini
      sudo chmod 600 /home/t8k/.secrets/certbot/cloudflare.ini
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
    sudo -u t8k mkdir -p /home/t8k/{src,t8k-go-server,etc/letsencrypt,lib/letsencrypt,log/letsencrypt,state,bin,scripts}
    sudo -u t8k mkdir -p /home/t8k/etc/pm2
    echo -e "${GREEN}✅ Production directories created at /home/t8k/${RESET}"
    ;;
  "dedicated")
    sudo -u t8k mkdir -p "/home/t8k/sites/${SITE_ID}"/{src,t8k-go-server,bin}
    sudo -u t8k mkdir -p /home/t8k/{etc/letsencrypt,lib/letsencrypt,log/letsencrypt,scripts,state}
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

  sudo -u t8k mkdir -p "$(dirname "$PORTS_CONFIG_FILE")"
  if ! sudo test -f "$PORTS_CONFIG_FILE"; then
    sudo -u t8k touch "$PORTS_CONFIG_FILE"
  fi

  # Read existing allocations from the file using sudo
  local ports_content
  ports_content=$(sudo cat "$PORTS_CONFIG_FILE")
  while IFS= read -r line; do
    if [[ "$line" =~ ^([a-zA-Z0-9_-]+)=([0-9]+),([0-9]+)$ ]]; then
      local go_port=${BASH_REMATCH[2]}
      local astro_port=${BASH_REMATCH[3]}
      used_go_ports+=("$go_port")
      used_astro_ports+=("$astro_port")
    fi
  done <<<"$ports_content"

  local current_site_id
  if [[ "${INSTALL_TYPE}" == "dedicated" ]]; then
    current_site_id="$SITE_ID"
  else
    current_site_id="main"
  fi

  local existing_go_port=""
  local existing_astro_port=""
  while IFS= read -r line; do
    if [[ "$line" =~ ^${current_site_id}=([0-9]+),([0-9]+)$ ]]; then
      existing_go_port=${BASH_REMATCH[1]}
      existing_astro_port=${BASH_REMATCH[2]}
      break
    fi
  done <<<"$ports_content"

  if [[ -n "$existing_go_port" ]] && [[ -n "$existing_astro_port" ]]; then
    ALLOCATED_GO_PORT="$existing_go_port"
    ALLOCATED_ASTRO_PORT="$existing_astro_port"
    echo -e "${GREEN}✅ Re-using existing allocated ports for ${current_site_id}: Go Port ${ALLOCATED_GO_PORT}, Astro Port ${ALLOCATED_ASTRO_PORT}${RESET}"
    return 0
  fi

  local go_port_start=10000
  [[ "${INSTALL_TYPE}" == "dedicated" ]] && go_port_start=10001
  for ((port = go_port_start; ; port++)); do
    [[ " ${used_go_ports[*]} " =~ " ${port} " ]] || {
      ALLOCATED_GO_PORT="$port"
      break
    }
  done

  local astro_port_start=20000
  [[ "${INSTALL_TYPE}" == "dedicated" ]] && astro_port_start=20001
  for ((port = astro_port_start; ; port++)); do
    [[ " ${used_astro_ports[*]} " =~ " ${port} " ]] || {
      ALLOCATED_ASTRO_PORT="$port"
      break
    }
  done

  if [[ -z "$ALLOCATED_GO_PORT" ]] || [[ -z "$ALLOCATED_ASTRO_PORT" ]]; then
    echo -e "${RED}❌ Failed to allocate Go or Astro ports.${RESET}"
    cleanup_lock
    exit 1
  fi

  echo -e "${GREEN}✅ Allocated new ports for ${current_site_id}: Go Port ${ALLOCATED_GO_PORT}, Astro Port ${ALLOCATED_ASTRO_PORT}${RESET}"

  local new_entry="${current_site_id}=${ALLOCATED_GO_PORT},${ALLOCATED_ASTRO_PORT}"
  sudo bash -c "echo '${new_entry}' >> ${PORTS_CONFIG_FILE}"
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
  sudo -i -u t8k git clone https://github.com/AtRiskMedia/tractstack-go.git "${src_dir}/tractstack-go"

  echo -e "${BLUE}Creating Go backend configuration...${RESET}"
  sudo -u t8k tee "${src_dir}/tractstack-go/.env" >/dev/null <<EOF
GO_BACKEND_PATH=${data_dir}/
PORT=${ALLOCATED_GO_PORT}
GIN_MODE=release
EOF

  if [[ "${INSTALL_TYPE}" == "multi" ]]; then
    cat <<EOF | sudo -u t8k tee -a "${src_dir}/tractstack-go/.env" >/dev/null
ENABLE_MULTI_TENANT=true
MAX_TENANTS=100
MAX_MEMORY_MB=2048
MAX_SESSIONS_PER_TENANT=1000
SSE_HEARTBEAT_INTERVAL_SECONDS=60
EOF
  else
    echo "ENABLE_MULTI_TENANT=false" | sudo -u t8k tee -a "${src_dir}/tractstack-go/.env" >/dev/null
  fi

  echo -e "${BLUE}Building Go backend...${RESET}"
  sudo -i -u t8k bash -c "cd '${src_dir}/tractstack-go' && go build -o '${bin_dir}/tractstack-go' ./cmd/tractstack-go"

  echo -e "${BLUE}Deploying operational scripts...${RESET}"
  sudo -i -u t8k bash -c "cp -r '${src_dir}/tractstack-go/pkg/scripts/'* /home/t8k/scripts/"
  sudo -i -u t8k bash -c "chmod +x /home/t8k/scripts/*.sh"

  echo -e "${GREEN}✅ Go backend deployed to ${bin_dir}/tractstack-go${RESET}"
}

# Deploy Astro frontend
deploy_astro_frontend() {
  echo -e "${BLUE}Deploying Astro frontend...${RESET}"

  local base_dir
  local tenant_id="default"
  if [[ "${INSTALL_TYPE}" == "dedicated" ]]; then
    base_dir="/home/t8k/sites/${SITE_ID}"
  else
    base_dir="/home/t8k"
  fi
  local src_dir="${base_dir}/src"
  local data_dir="${base_dir}/t8k-go-server"

  echo -e "${BLUE}Creating Astro frontend project...${RESET}"
  if [[ "${NON_INTERACTIVE}" == true ]]; then
    sudo -i -u t8k bash -c "cd '${src_dir}' && pnpm create astro@latest my-tractstack --template minimal --typescript strict --install --git --yes"
  else
    sudo -i -u t8k bash -c "cd '${src_dir}' && pnpm create astro@latest my-tractstack --template minimal --typescript strict --install --git" </dev/tty
  fi

  echo -e "${BLUE}Installing TractStack integration...${RESET}"
  sudo -i -u t8k bash -c "cd '${src_dir}/my-tractstack' && pnpm add astro-tractstack@latest"

  echo -e "${BLUE}Creating Astro frontend configuration...${RESET}"
  sudo -u t8k tee "${src_dir}/my-tractstack/.env" >/dev/null <<EOF
PRIVATE_GO_BACKEND_PATH=${data_dir}/
PUBLIC_GO_BACKEND=http://localhost:${ALLOCATED_GO_PORT}
PUBLIC_TENANTID=${tenant_id}
EOF

  if [[ "${INSTALL_TYPE}" == "multi" ]]; then
    echo "PUBLIC_ENABLE_MULTI_TENANT=true" | sudo -u t8k tee -a "${src_dir}/my-tractstack/.env" >/dev/null
  else
    echo "PUBLIC_ENABLE_MULTI_TENANT=false" | sudo -u t8k tee -a "${src_dir}/my-tractstack/.env" >/dev/null
  fi

  echo -e "${BLUE}Running TractStack setup...${RESET}"
  sudo -i -u t8k bash -c "cd '${src_dir}/my-tractstack' && npx create-tractstack" </dev/tty

  echo -e "${BLUE}Building Astro frontend for production...${RESET}"
  sudo -i -u t8k bash -c "cd '${src_dir}/my-tractstack' && pnpm build"

  if ! sudo test -d "${src_dir}/my-tractstack/dist"; then
    echo -e "${RED}❌ Astro build failed - no dist directory found${RESET}"
    cleanup_lock
    exit 1
  fi

  echo -e "${GREEN}✅ Astro frontend deployed and built at ${src_dir}/my-tractstack${RESET}"
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

  echo -e "${BLUE}Creating system user 't8k'...${RESET}"
  case $PACKAGE_MANAGER in
  apt)
    # Use the more feature-rich adduser on Debian-based systems
    sudo adduser --system --group --shell /bin/bash --home /home/t8k t8k
    ;;
  dnf | pacman | zypper | apk | *)
    # Use the more standard useradd on other systems
    sudo useradd --system --create-home --shell /bin/bash -m t8k
    ;;
  esac

  echo -e "${YELLOW}Please set a password for the 't8k' user for maintenance.${RESET}"
  sudo passwd t8k </dev/tty

  sudo bash -c 'echo "t8k ALL=(root) NOPASSWD: /bin/systemctl restart tractstack-go*, /bin/systemctl reload tractstack-go*, /usr/bin/systemctl restart tractstack-go*, /usr/bin/systemctl reload tractstack-go*" > /etc/sudoers.d/t8k-services'
  sudo chmod 440 /etc/sudoers.d/t8k-services

  echo -e "${GREEN}✅ User 't8k' created successfully${RESET}"
}

# Sets up the build environment for the t8k user by creating a .profile
setup_t8k_environment() {
  echo -e "${BLUE}Configuring build environment for t8k user...${RESET}"

  local go_path node_path pnpm_path npm_path git_path augmented_path
  go_path=$(dirname "$(command -v go)")
  node_path=$(dirname "$(command -v node)")
  pnpm_path=$(dirname "$(command -v pnpm)")
  npm_path=$(dirname "$(command -v npm)")

  # Deduplicate and form the augmented PATH.
  augmented_path=$(echo "${go_path}:${node_path}:${pnpm_path}:${npm_path}:${PATH}" | awk -v RS=: '{ if (!seen[$0]++) { if (NR > 1) printf(":"); printf("%s", $0) } }')

  # Create /home/t8k/.profile
  sudo -u t8k tee /home/t8k/.profile >/dev/null <<EOF
# This file is automatically generated by the TractStack installer.
export PATH="${augmented_path}"
export PM2_HOME="/home/t8k/.pm2"
EOF

  sudo chown t8k:t8k /home/t8k/.profile
  echo -e "${GREEN}✅ t8k user environment configured.${RESET}"
}

# Setup SSL certificates
setup_ssl_certificates() {
  dry_run_flag=""
  if [[ "$DRY_RUN" == "true" ]]; then
    echo -e "${BLUE}Setting up SSL certificates...${RESET} [SIMULATION]"
    dry_run_flag="--dry-run"
  else
    echo -e "${BLUE}Setting up SSL certificates...${RESET}"
  fi

  # Install python3-venv if needed, then create venv and install certbot
  if ! sudo test -f /home/t8k/certbot_venv/bin/certbot; then
    echo -e "${BLUE}Installing certbot in virtual environment...${RESET}"

    # Install venv package based on OS
    case $PACKAGE_MANAGER in
    apt)
      sudo apt update && sudo apt install -y python3-venv python3-pip
      ;;
    dnf)
      sudo dnf install -y python3-venv python3-pip
      ;;
    pacman)
      sudo pacman -S --noconfirm python python-pip
      ;;
    *)
      echo -e "${YELLOW}Cannot auto-install python3-venv. Please install it manually.${RESET}"
      return 1
      ;;
    esac

    # Create venv and install certbot
    sudo -i -u t8k bash -c "
     python3 -m venv /home/t8k/certbot_venv && \
     source /home/t8k/certbot_venv/bin/activate && \
     pip install certbot certbot-dns-cloudflare
   "

    # Verify installation worked
    if ! sudo test -f /home/t8k/certbot_venv/bin/certbot; then
      echo -e "${RED}❌ Certbot venv installation failed${RESET}"
      cleanup_lock
      exit 1
    fi

    echo -e "${GREEN}✅ Certbot installed in /home/t8k/certbot_venv${RESET}"
  fi

  sudo -u t8k mkdir -p /home/t8k/etc/letsencrypt /home/t8k/lib/letsencrypt /home/t8k/log/letsencrypt

  local primary_cert_domains="-d ${DOMAIN} -d *.${DOMAIN}"
  local primary_cert_path="/home/t8k/etc/letsencrypt/live/${DOMAIN}"

  if sudo test -f "${primary_cert_path}/fullchain.pem"; then
    echo -e "${GREEN}✅ SSL certificates already exist, skipping request.${RESET}"
    return 0
  fi

  echo -e "${BLUE}Requesting SSL certificates for: ${primary_cert_domains}${RESET}"

  if sudo test -f "/home/t8k/.secrets/certbot/cloudflare.ini"; then
    echo -e "${GREEN}Using Cloudflare DNS automation${RESET}"
    sudo -i -u t8k bash -c "
     source /home/t8k/certbot_venv/bin/activate && \
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
    sudo -i -u t8k bash -c "
     source /home/t8k/certbot_venv/bin/activate && \
     certbot certonly --manual --preferred-challenges dns \
       --config-dir /home/t8k/etc/letsencrypt \
       --work-dir /home/t8k/lib/letsencrypt \
       --logs-dir /home/t8k/log/letsencrypt \
       --agree-tos --email admin@${DOMAIN} \
       $dry_run_flag \
       ${primary_cert_domains}
   " </dev/tty
  fi
  echo -e "${GREEN}✅ SSL certificate request completed${RESET}"
}

# Configure nginx
configure_nginx() {
  echo -e "${BLUE}Configuring nginx...${RESET}"

  if ! systemctl is-active --quiet nginx &>/dev/null; then
    sudo systemctl start nginx
    sudo systemctl enable nginx
  fi

  local config_name
  if [[ "${INSTALL_TYPE}" == "dedicated" ]]; then
    config_name="t8k-${SITE_ID}"
  else
    config_name="t8k-default"
  fi

  sudo mkdir -p /etc/nginx/sites-available /etc/nginx/sites-enabled
  local config_file="/etc/nginx/sites-available/${config_name}.conf"

  local nginx_config
  case "${INSTALL_TYPE}" in
  "prod")
    nginx_config=$(
      cat <<EOF
server {
    listen 80;
    server_name ${DOMAIN} www.${DOMAIN};
    return 301 https://\$host\$request_uri;
}

server {
    listen 443 ssl http2;
    server_name ${DOMAIN} www.${DOMAIN};
    ssl_certificate /home/t8k/etc/letsencrypt/live/${DOMAIN}/fullchain.pem;
    ssl_certificate_key /home/t8k/etc/letsencrypt/live/${DOMAIN}/privkey.pem;
    
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
    )
    ;;
  "multi")
    nginx_config=$(
      cat <<EOF
server {
    listen 80;
    server_name ${DOMAIN} www.${DOMAIN} *.${DOMAIN};
    return 301 https://\$host\$request_uri;
}

server {
    listen 443 ssl http2;
    server_name ${DOMAIN} www.${DOMAIN} *.${DOMAIN};
    ssl_certificate /home/t8k/etc/letsencrypt/live/${DOMAIN}/fullchain.pem;
    ssl_certificate_key /home/t8k/etc/letsencrypt/live/${DOMAIN}/privkey.pem;
    
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
    )
    ;;
  "dedicated")
    nginx_config=$(
      cat <<EOF
server {
    listen 80;
    server_name ${DOMAIN} www.${DOMAIN};
    return 301 https://\$host\$request_uri;
}

server {
    listen 443 ssl http2;
    server_name ${DOMAIN} www.${DOMAIN};
    ssl_certificate /home/t8k/etc/letsencrypt/live/${DOMAIN}/fullchain.pem;
    ssl_certificate_key /home/t8k/etc/letsencrypt/live/${DOMAIN}/privkey.pem;
    
    location / {
        proxy_pass http://localhost:${ALLOCATED_ASTRO_PORT};
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
    
    location /media/ {
        alias /home/t8k/sites/${SITE_ID}/t8k-go-server/config/default/media/;
    }
}
EOF
    )
    ;;
  *)
    echo -e "${RED}⛌ Unknown install type: ${INSTALL_TYPE}${RESET}"
    cleanup_lock
    exit 1
    ;;
  esac

  echo "$nginx_config" | sudo tee "$config_file" >/dev/null
  echo -e "${BLUE}Creating nginx site configuration: ${config_name}.conf${RESET}"
  sudo ln -sf "$config_file" "/etc/nginx/sites-enabled/${config_name}.conf"
  if ! sudo nginx -t; then
    echo -e "${RED}⛌ nginx configuration test failed${RESET}"
    sudo rm -f "/etc/nginx/sites-enabled/${config_name}.conf"
    cleanup_lock
    exit 1
  fi
  sudo systemctl reload nginx
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
  local service_content

  case "${INSTALL_TYPE}" in
  "prod" | "multi")
    service_name="tractstack-go"
    service_file="/etc/systemd/system/${service_name}.service"
    binary_path="/home/t8k/bin/tractstack-go"
    working_dir="/home/t8k/src/tractstack-go"
    data_dir="/home/t8k/t8k-go-server"
    env_multi=""
    if [[ "${INSTALL_TYPE}" == "multi" ]]; then
      env_multi="Environment=ENABLE_MULTI_TENANT=true"
    fi
    service_content=$(
      cat <<EOF
[Unit]
Description=TractStack Go Backend
After=network.target
[Service]
Type=simple
User=t8k
Group=t8k
WorkingDirectory=${working_dir}
Environment=GO_BACKEND_PATH=${data_dir}/
Environment=PORT=${ALLOCATED_GO_PORT}
Environment=GIN_MODE=release
${env_multi}
ExecStart=${binary_path}
Restart=always
[Install]
WantedBy=multi-user.target
EOF
    )
    ;;
  "dedicated")
    service_name="tractstack-go@${SITE_ID}"
    # Use a template file for dedicated instances
    service_file="/etc/systemd/system/tractstack-go@.service"
    binary_path="/home/t8k/sites/%i/bin/tractstack-go"
    working_dir="/home/t8k/sites/%i/src/tractstack-go"
    data_dir="/home/t8k/sites/%i/t8k-go-server"
    service_content=$(
      cat <<EOF
[Unit]
Description=TractStack Go Backend (Site: %i)
After=network.target
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
[Install]
WantedBy=multi-user.target
EOF
    )
    ;;
  *)
    echo -e "${RED}⛌ Unknown install type: ${INSTALL_TYPE}${RESET}"
    cleanup_lock
    exit 1
    ;;
  esac

  echo -e "${BLUE}Creating systemd service: ${service_name}${RESET}"
  echo "$service_content" | sudo tee "$service_file" >/dev/null

  setup_build_watcher

  echo -e "${BLUE}Reloading systemd daemon...${RESET}"
  sudo systemctl daemon-reload

  echo -e "${BLUE}Enabling and starting ${service_name}...${RESET}"
  sudo systemctl enable "$service_name"
  sudo systemctl start "$service_name"

  if sudo systemctl is-active --quiet "$service_name"; then
    echo -e "${GREEN}✅ ${service_name} is running${RESET}"
  else
    echo -e "${RED}⛌ ${service_name} failed to start${RESET}"
    sudo journalctl -u "$service_name" -n 20 --no-pager
    cleanup_lock
    exit 1
  fi
}

# Setup build system file watcher
setup_build_watcher() {
  local path_unit="/etc/systemd/system/t8k-build-watcher.path"
  local service_unit="/etc/systemd/system/t8k-build-watcher.service"

  if sudo systemctl list-unit-files | grep -q "t8k-build-watcher.path"; then
    echo -e "${BLUE}Build system watcher already configured, skipping...${RESET}"
    return 0
  fi

  echo -e "${BLUE}Setting up build system file watcher...${RESET}"
  sudo tee "$path_unit" >/dev/null <<EOF
[Unit]
Description=TractStack Build Watcher
[Path]
PathModified=/home/t8k/state
Unit=t8k-build-watcher.service
[Install]
WantedBy=multi-user.target
EOF
  sudo tee "$service_unit" >/dev/null <<EOF
[Unit]
Description=TractStack Build Concierge
[Service]
Type=oneshot
User=t8k
Group=t8k
ExecStart=/home/t8k/scripts/t8k-concierge.sh
[Install]
WantedBy=multi-user.target
EOF

  echo -e "${GREEN}✅ Created build watcher systemd units${RESET}"
  sudo systemctl enable t8k-build-watcher.path
  sudo systemctl start t8k-build-watcher.path
}

# Setup PM2 ecosystem
setup_pm2_ecosystem() {
  echo -e "${BLUE}Setting up PM2 ecosystem...${RESET}"

  local ecosystem_file="/home/t8k/etc/pm2/ecosystem.config.js"

  # Create ecosystem file as the t8k user
  sudo -u t8k tee "$ecosystem_file" >/dev/null <<'EOF'
const fs = require('fs');
const path = require('path');
const portsFile = '/home/t8k/etc/t8k-ports.conf';
const apps = [];
if (fs.existsSync(portsFile)) {
  const portsContent = fs.readFileSync(portsFile, 'utf8');
  portsContent.trim().split('\n').forEach(line => {
    if (!line.includes('=')) return;
    const [siteId, ports] = line.split('=');
    const [, astroPort] = ports.split(',');
    const appPath = (siteId === 'main') ? '/home/t8k/src/my-tractstack' : `/home/t8k/sites/${siteId}/src/my-tractstack`;
    const appName = `astro-${siteId}`;
    if (fs.existsSync(path.join(appPath, 'dist/server/entry.mjs'))) {
      apps.push({ name: appName, script: 'dist/server/entry.mjs', cwd: appPath, env: { NODE_ENV: 'production', PORT: astroPort, HOST: '0.0.0.0' } });
    }
  });
}
module.exports = { apps };
EOF

  echo -e "${BLUE}Starting or reloading PM2 ecosystem...${RESET}"
  sudo -i -u t8k pm2 startOrReload "$ecosystem_file"

  echo -e "${BLUE}Configuring PM2 startup service...${RESET}"
  # This command must be run as root to create the systemd file, but we tell it to generate a service for user t8k
  local pm2_path
  pm2_path=$(command -v pm2)
  sudo env PATH=$PATH:"$(dirname "$pm2_path")" "$pm2_path" startup systemd -u t8k --hp /home/t8k

  echo -e "${BLUE}Saving process list for reboot...${RESET}"
  sudo -i -u t8k pm2 save

  echo -e "${GREEN}✅ PM2 ecosystem configured and running${RESET}"
}

# Quick install implementation
quick_install() {
  USER=$(whoami)
  INSTALL_DIR="/home/$USER/t8k"
  echo -e "${BLUE}Starting TractStack quick installation...${RESET}"
  echo -e "Install directory: ${WHITE}$INSTALL_DIR${RESET}"
  if [[ -d "$INSTALL_DIR" ]]; then
    echo -e "${RED}❌ Directory $INSTALL_DIR already exists!${RESET}"
    cleanup_lock
    exit 1
  fi

  mkdir -p "$INSTALL_DIR/src"
  cd "$INSTALL_DIR/src"
  echo -e "${BLUE}Cloning and building Go backend...${RESET}"
  git clone https://github.com/AtRiskMedia/tractstack-go.git
  cd tractstack-go
  echo "GO_BACKEND_PATH=/home/$USER/t8k/t8k-go-server/" >.env
  go build -o tractstack-go ./cmd/tractstack-go

  echo -e "${BLUE}Creating Astro frontend project...${RESET}"
  cd "$INSTALL_DIR/src"
  pnpm create astro@latest my-tractstack --template minimal --typescript strict --install --yes --no-git </dev/tty

  echo -e "${BLUE}Installing and configuring TractStack integration...${RESET}"
  cd my-tractstack
  pnpm add astro-tractstack@latest
  echo "PRIVATE_GO_BACKEND_PATH=/home/$USER/t8k/t8k-go-server/" >.env
  echo "PUBLIC_GO_BACKEND=http://localhost:8080" >>.env
  echo "PUBLIC_TENANTID=default" >>.env
  echo "ENABLE_MULTI_TENANT=false" >>.env
  npx create-tractstack </dev/tty

  echo -e "${BLUE}Building Astro project for whitelist extraction...${RESET}"
  pnpm build

  # Ensure config directory exists
  mkdir -p "/home/$USER/t8k/t8k-go-server/config/default"
  echo -e "${BLUE}Extracting Tailwind whitelist...${RESET}"
  # Ensure config directory exists
  mkdir -p "/home/$USER/t8k/t8k-go-server/config/default"
  # Extract whitelist from built assets
  python3 "$INSTALL_DIR/src/tractstack-go/pkg/scripts/extractTailwindWhitelist.py" \
    "$INSTALL_DIR/src/my-tractstack/dist" \
    "/home/$USER/t8k/t8k-go-server/config/default/tailwindWhitelist.json"

  echo -e "\n${GREEN}🎉 TractStack installation complete!${RESET}\n"
  echo -e "${WHITE}To start your TractStack site:${RESET}"
  echo -e "1. Start Go backend:   ${BLUE}cd $INSTALL_DIR/src/tractstack-go && ./tractstack-go${RESET}"
  echo -e "2. Start dev server:   ${BLUE}cd $INSTALL_DIR/src/my-tractstack && pnpm dev${RESET}"
  echo -e "\nYour site will be at: ${BLUE}http://localhost:4321${RESET}\n"
}

# Production install orchestration
production_install() {
  echo -e "${BLUE}Starting TractStack production installation...${RESET}"
  echo -e "Type: ${WHITE}${INSTALL_TYPE}${RESET}, Domain: ${WHITE}${DOMAIN}${RESET}"
  [[ "${INSTALL_TYPE}" == "dedicated" ]] && echo -e "Site ID: ${WHITE}${SITE_ID}${RESET}"

  check_existing_installation

  create_t8k_user
  setup_t8k_environment
  setup_directories

  detect_cloudflare_secrets
  setup_ssl_certificates

  allocate_ports

  deploy_go_backend
  deploy_astro_frontend

  configure_nginx
  configure_systemd_services
  setup_pm2_ecosystem
  extract_initial_whitelist

  echo -e "\n${GREEN}🎉 TractStack production installation complete!${RESET}\n"
  echo -e "${WHITE}Your site is available at:${RESET}"
  echo -e "   ${BLUE}https://${DOMAIN}${RESET} (Production domain)"
  echo -e "   ${BLUE}http://localhost:${ALLOCATED_ASTRO_PORT}${RESET} (Direct Astro access)"
  echo -e "   ${BLUE}http://localhost:${ALLOCATED_GO_PORT}${RESET} (Go backend API)"
}

# More robust go version check
check_go_version() {
  local full_version
  full_version=$(go version 2>/dev/null | grep -o 'go[0-9][.0-9]*' | head -1 | sed 's/go//')
  if [[ -z "$full_version" ]]; then
    echo -e "${RED}❌ Could not determine Go version. Is it in your PATH?${RESET}"
    return 1
  fi

  local major minor
  major=$(echo "$full_version" | cut -d. -f1)
  minor=$(echo "$full_version" | cut -d. -f2)

  if [[ $major -lt 1 ]] || [[ $major -eq 1 && $minor -lt 22 ]]; then
    echo -e "${RED}❌ Go version $full_version found, but TractStack requires Go 1.22+${RESET}"
    return 1
  fi
  echo -e "${GREEN}✅ Go $full_version found${RESET}"
  return 0
}

# Checks for essential developer dependencies
check_essential_prerequisites() {
  echo -e "${BLUE}Checking essential prerequisites...${RESET}"
  local MISSING_DEPS=()
  if ! command -v go &>/dev/null; then MISSING_DEPS+=("Go"); elif ! check_go_version; then MISSING_DEPS+=("Go 1.22+"); fi
  if ! command -v node &>/dev/null; then MISSING_DEPS+=("Node.js"); fi
  if ! command -v git &>/dev/null; then MISSING_DEPS+=("Git"); fi
  if ! command -v npm &>/dev/null; then MISSING_DEPS+=("npm"); fi
  if ! command -v pnpm &>/dev/null; then MISSING_DEPS+=("pnpm"); fi
  if ! command -v python3 &>/dev/null; then MISSING_DEPS+=("python3"); fi
  if ! python3 -c "import bs4" &>/dev/null 2>&1; then MISSING_DEPS+=("python3-bs4"); fi

  if [[ ${#MISSING_DEPS[@]} -gt 0 ]]; then
    echo -e "${RED}Error: Missing essential developer dependencies:${RESET}"
    for dep in "${MISSING_DEPS[@]}"; do
      echo -e "  ${RED}⌘ ${dep}${RESET}"
      show_install_instructions "$dep"
    done
    cleanup_lock
    exit 1
  fi

  echo -e "${GREEN}✅ All essential prerequisites found.${RESET}"
}

# Checks for production system dependencies
check_production_prerequisites() {
  echo -e "${BLUE}Checking production prerequisites...${RESET}"
  local MISSING_DEPS=()
  if ! [[ -x /usr/sbin/nginx ]]; then MISSING_DEPS+=("nginx"); fi
  if ! command -v pm2 &>/dev/null; then MISSING_DEPS+=("pm2"); fi
  if [[ "$OS" != "macos" ]] && ! command -v systemctl &>/dev/null; then MISSING_DEPS+=("systemd"); fi

  if [[ ${#MISSING_DEPS[@]} -gt 0 ]]; then
    echo -e "${RED}Error: Missing production dependencies:${RESET}"
    for dep in "${MISSING_DEPS[@]}"; do
      echo -e "  ${RED}❌ ${dep}${RESET}"
      show_install_instructions "$dep"
    done
    cleanup_lock
    exit 1
  fi
  echo -e "${GREEN}✅ All production prerequisites found.${RESET}"
}

# Function to extract initial tailwind whitelist
extract_initial_whitelist() {
  echo -e "${BLUE}Extracting Tailwind whitelist from built assets...${RESET}"

  local base_dir
  if [[ "${INSTALL_TYPE}" == "dedicated" ]]; then
    base_dir="/home/t8k/sites/${SITE_ID}"
  else
    base_dir="/home/t8k"
  fi

  local astro_dist_dir="${base_dir}/src/my-tractstack/dist"
  local whitelist_output="${base_dir}/t8k-go-server/config/default/tailwindWhitelist.json"

  # Ensure config directory exists
  sudo -u t8k mkdir -p "${base_dir}/t8k-go-server/config/default"

  # Verify dist directory exists
  if ! sudo test -d "$astro_dist_dir"; then
    echo -e "${RED}⌘ Astro dist directory not found: $astro_dist_dir${RESET}"
    return 1
  fi

  # Run extraction as t8k user
  if sudo -u t8k python3 /home/t8k/scripts/extractTailwindWhitelist.py "$astro_dist_dir" "$whitelist_output"; then
    echo -e "${GREEN}✅ Tailwind whitelist extracted successfully${RESET}"
  else
    echo -e "${RED}⌘ Failed to extract Tailwind whitelist${RESET}"
    return 1
  fi

  return 0
}

# Main execution logic
main() {
  # Self-download logic for curl | bash execution
  if [[ ! -t 0 ]] && [[ ! -f "$0" || "$0" == "bash" ]]; then
    # Create user-specific temp directory
    user_temp_dir="$HOME/.tmp"
    mkdir -p "$user_temp_dir"

    # Create temp script in user directory instead of /tmp
    temp_script=$(mktemp "$user_temp_dir/t8k-install.XXXXXX")

    # Ensure lock is removed before exec
    rm -f "$LOCK_FILE"

    # Download to user temp directory
    curl -fsSL https://get.tractstack.com >"$temp_script"
    chmod +x "$temp_script"

    # Clean up temp file after exec (trap ensures cleanup even if script fails)
    trap "rm -f '$temp_script'" EXIT

    exec "$temp_script" "$@"
  fi

  show_header "$@"
  detect_os_and_package_manager

  if [[ "$(whoami)" == "t8k" ]]; then
    echo -e "${RED}❌ This installer cannot be run as the 't8k' service user.${RESET}"
    exit 1
  fi

  # Non-interactive / CLI mode
  if ! is_interactive "$@"; then
    parse_cli_args "$@"
    if [[ "${INSTALL_TYPE}" == "quick" ]]; then
      check_essential_prerequisites
      quick_install
    else
      if [[ "$(whoami)" == "root" ]]; then
        echo -e "${RED}❌ For safety, please run production installs as a regular user with sudo access, not as root.${RESET}"
        exit 1
      fi
      if ! has_sudo; then
        echo -e "${RED}❌ Sudo access is required for production installs.${RESET}"
        exit 1
      fi
      check_essential_prerequisites
      check_production_prerequisites
      production_install
    fi
  # Interactive mode
  else
    if [[ "$(whoami)" == "root" ]]; then
      echo -e "${RED}❌ For safety, please run interactive installs as a regular user with sudo access, not as root.${RESET}"
      exit 1
    fi

    echo -e "${BLUE}Choose your installation goal:${RESET}"
    echo "1) Quick Install (for local development)"
    echo "2) Production Install (to set up a live server)"
    read -p "Enter choice [1-2]: " choice </dev/tty

    case $choice in
    1)
      INSTALL_TYPE="quick"
      check_essential_prerequisites
      quick_install
      ;;
    2)
      # CORRECTED LOGIC: Check all prerequisites BEFORE asking questions.
      check_essential_prerequisites
      check_production_prerequisites
      if ! has_sudo; then
        echo -e "${RED}❌ Sudo access is required for production installs.${RESET}"
        exit 1
      fi

      echo -e "${BLUE}Choose production installation type:${RESET}"
      echo "1) Production single-tenant"
      echo "2) Production multi-tenant"
      echo "3) Dedicated tenant"
      read -p "Enter choice [1-3]: " prod_choice </dev/tty

      case $prod_choice in
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
        echo -e "${RED}Invalid choice.${RESET}"
        exit 1
        ;;
      esac

      production_install
      ;;
    *)
      echo -e "${RED}Invalid choice. Please try again.${RESET}"
      ;;
    esac
  fi

  cleanup_lock
}

# Run main function, passing all arguments to it
main "$@"
exit 0
