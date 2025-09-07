#!/bin/bash

# TractStack v2 Uninstall Script
# Removes TractStack installations based on command line arguments
#
# Usage:
#   ./t8k-uninstall.sh main              # Remove main/prod/multi installation
#   ./t8k-uninstall.sh site {siteId}     # Remove dedicated site
#   ./t8k-uninstall.sh all               # Remove everything including t8k user
#   ./t8k-uninstall.sh --non-interactive # Run without prompts or ASCII art

set -euo pipefail

# Colors for output
GREEN='\033[32m'
BLUE='\033[34m'
YELLOW='\033[33m'
RED='\033[31m'
WHITE='\033[97m'
RESET='\033[0m'

# Global variables
NON_INTERACTIVE=false
UNINSTALL_TARGET=""
SITE_ID=""
PORTS_CONFIG_FILE="/home/t8k/etc/t8k-ports.conf"

# Show TractStack ASCII art (only in interactive mode)
show_header() {
  if [[ "$NON_INTERACTIVE" == true ]]; then
    return
  fi

  echo -e "${RED}"
  echo ' ▄██▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄██▄▄▄▄▄▄▄██▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄ ▄▄▄'
  echo '  ██  ██ ██ ▀▀ ██ ██ ▀▀ ██ ██ ▀▀ ██ ▀▀ ██ ██ ▀▀ ██ ██'
  echo '  ██  ██▀█▄ ██▀██ ██ ▄▄ ██ ▀▀▀██ ██ ██▀██ ██ ▄▄ ██▀█▄'
  echo '  ██  ██ ██ ██▄██ ██▄██ ██ ██▄██ ██ ██▄██ ██▄██ ██ ██'
  echo '   ▀▀                   ▀▀       ▀▀             ▀▀ ▀▀▀'
  echo -e "${WHITE}"
  echo '  UNINSTALLER'
  echo -e "${RESET}"
  echo
}

# Basic logging function
log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

log_error() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $*" >&2
}

# Check if running as root (required for production uninstalls)
check_root() {
  if [[ "$(whoami)" != "root" ]]; then
    echo -e "${RED}Production uninstalls must be run as root${RESET}"
    echo "Please run with: sudo $0 $*"
    exit 1
  fi
}

# Parse command line arguments
parse_args() {
  while [[ $# -gt 0 ]]; do
    case $1 in
    --non-interactive)
      NON_INTERACTIVE=true
      shift
      ;;
    main)
      UNINSTALL_TARGET="main"
      shift
      ;;
    site)
      UNINSTALL_TARGET="site"
      if [[ -z "${2:-}" ]]; then
        echo -e "${RED}Site ID required after 'site'${RESET}"
        exit 1
      fi
      SITE_ID="$2"
      shift 2
      ;;
    all)
      UNINSTALL_TARGET="all"
      shift
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
}

# Show usage
show_usage() {
  echo "TractStack v2 Uninstaller"
  echo
  echo "Usage: $0 [TARGET] [OPTIONS]"
  echo
  echo "Targets:"
  echo "  main                       Remove main/prod/multi installation"
  echo "  site <site-id>            Remove dedicated site installation"
  echo "  all                       Remove everything including t8k user"
  echo
  echo "Options:"
  echo "  --non-interactive         Run without prompts or ASCII art"
  echo "  --help, -h               Show this help message"
  echo
  echo "Examples:"
  echo "  $0 main"
  echo "  $0 site mysite"
  echo "  $0 all"
  echo "  $0 --non-interactive all"
}

# Read installed sites from ports config
read_installed_sites() {
  local sites=()

  if [[ ! -f "$PORTS_CONFIG_FILE" ]]; then
    echo "${sites[@]}"
    return
  fi

  while IFS="=" read -r site_id port_pair; do
    if [[ -n "$site_id" ]] && [[ "$site_id" =~ ^[a-zA-Z0-9_-]+$ ]]; then
      sites+=("$site_id")
    fi
  done <"$PORTS_CONFIG_FILE"

  echo "${sites[@]}"
}

# Interactive target selection
choose_uninstall_target() {
  local sites
  read -ra sites <<<"$(read_installed_sites)"

  if [[ ${#sites[@]} -eq 0 ]]; then
    echo -e "${YELLOW}No TractStack installations found${RESET}"
    echo -e "${BLUE}You can still choose:${RESET}"
    echo "*) all (remove any remaining TractStack artifacts)"
    echo
    read -p "Enter choice: " choice

    case "$choice" in
    "*" | "all")
      UNINSTALL_TARGET="all"
      ;;
    *)
      echo "Nothing to uninstall"
      exit 0
      ;;
    esac
    return
  fi

  echo -e "${BLUE}Choose what to uninstall:${RESET}"

  local choice_num=1
  local has_main=false

  # Show main option if it exists
  for site in "${sites[@]}"; do
    if [[ "$site" == "main" ]]; then
      echo "$choice_num) main (production/multi-tenant)"
      has_main=true
      ((choice_num++))
      break
    fi
  done

  # Show dedicated sites
  for site in "${sites[@]}"; do
    if [[ "$site" != "main" ]]; then
      echo "$choice_num) site: $site"
      ((choice_num++))
    fi
  done

  echo "*) all (remove everything including t8k user)"
  echo

  read -p "Enter choice: " choice

  case "$choice" in
  1)
    if [[ "$has_main" == true ]]; then
      UNINSTALL_TARGET="main"
    else
      # First dedicated site
      for site in "${sites[@]}"; do
        if [[ "$site" != "main" ]]; then
          UNINSTALL_TARGET="site"
          SITE_ID="$site"
          break
        fi
      done
    fi
    ;;
  [2-9]*)
    local target_index=$((choice - 1))
    if [[ "$has_main" == true ]]; then
      target_index=$((choice - 2))
    fi

    local dedicated_sites=()
    for site in "${sites[@]}"; do
      if [[ "$site" != "main" ]]; then
        dedicated_sites+=("$site")
      fi
    done

    if [[ $target_index -ge 0 ]] && [[ $target_index -lt ${#dedicated_sites[@]} ]]; then
      UNINSTALL_TARGET="site"
      SITE_ID="${dedicated_sites[$target_index]}"
    else
      echo -e "${RED}Invalid choice${RESET}"
      choose_uninstall_target
      return
    fi
    ;;
  "*" | "all")
    UNINSTALL_TARGET="all"
    ;;
  *)
    echo -e "${RED}Invalid choice${RESET}"
    choose_uninstall_target
    ;;
  esac
}

# Confirmation prompt
confirm_uninstall() {
  if [[ "$NON_INTERACTIVE" == true ]]; then
    return 0
  fi

  local target_desc=""
  case "$UNINSTALL_TARGET" in
  "main")
    target_desc="main TractStack installation (prod/multi)"
    ;;
  "site")
    target_desc="dedicated site: $SITE_ID"
    ;;
  "all")
    target_desc="ALL TractStack installations and the t8k user"
    ;;
  esac

  echo -e "${YELLOW}WARNING: This will permanently remove ${target_desc}${RESET}"
  read -p "Are you sure? [y/N]: " confirm

  if [[ ! "$confirm" =~ ^[Yy] ]]; then
    echo "Uninstall cancelled"
    exit 0
  fi
}

# Improved remove_systemd_services function with better error handling
remove_systemd_services() {
  local target="$1"
  local site_id="${2:-}"

  case "$target" in
  "main")
    log "Stopping and removing main systemd service"
    if systemctl is-active --quiet tractstack-go.service 2>/dev/null; then
      systemctl stop tractstack-go.service
    fi
    if systemctl is-enabled --quiet tractstack-go.service 2>/dev/null; then
      systemctl disable tractstack-go.service
    fi
    rm -f /etc/systemd/system/tractstack-go.service
    ;;
  "site")
    log "Stopping and removing dedicated site systemd service: $site_id"
    local service_name="tractstack-go@${site_id}.service"

    if systemctl is-active --quiet "$service_name" 2>/dev/null; then
      log "Stopping service: $service_name"
      systemctl stop "$service_name"
    fi

    if systemctl is-enabled --quiet "$service_name" 2>/dev/null; then
      log "Disabling service: $service_name"
      systemctl disable "$service_name"
    fi

    if ! check_remaining_installations; then
      log "No remaining installations, removing template service file"
      rm -f /etc/systemd/system/tractstack-go@.service
    fi
    ;;
  "all")
    log "Stopping and removing all systemd services"

    for service in $(systemctl list-units --type=service --state=active "tractstack-go*" --no-legend | awk '{print $1}'); do
      log "Stopping service: $service"
      systemctl stop "$service" || true
    done

    for service in $(systemctl list-unit-files "tractstack-go*" --no-legend | awk '{print $1}'); do
      if systemctl is-enabled --quiet "$service" 2>/dev/null; then
        log "Disabling service: $service"
        systemctl disable "$service" || true
      fi
    done

    rm -f /etc/systemd/system/tractstack-go*.service
    find /etc/systemd/system -name "*tractstack-go*" -type l -delete 2>/dev/null || true

    if systemctl is-active --quiet t8k-build-watcher.path 2>/dev/null; then
      systemctl stop t8k-build-watcher.path
    fi
    if systemctl is-enabled --quiet t8k-build-watcher.path 2>/dev/null; then
      systemctl disable t8k-build-watcher.path
    fi
    if systemctl is-active --quiet t8k-build-watcher.service 2>/dev/null; then
      systemctl stop t8k-build-watcher.service
    fi
    if systemctl is-enabled --quiet t8k-build-watcher.service 2>/dev/null; then
      systemctl disable t8k-build-watcher.service
    fi
    rm -f /etc/systemd/system/t8k-build-watcher.*
    ;;
  esac

  systemctl daemon-reload
}

# Stop and remove PM2 processes
remove_pm2_processes() {
  local target="$1"
  local site_id="${2:-}"

  if ! id "t8k" &>/dev/null; then
    log "User t8k does not exist, skipping PM2 cleanup"
    return 0
  fi

  case "$target" in
  "main")
    log "Removing PM2 process: astro-main"
    if sudo -u t8k pm2 list | grep -q "astro-main" 2>/dev/null; then
      sudo -u t8k pm2 delete astro-main || true
    fi
    ;;
  "site")
    log "Removing PM2 process: astro-$site_id"
    local process_name="astro-${site_id}"
    if sudo -u t8k pm2 list | grep -q "$process_name" 2>/dev/null; then
      sudo -u t8k pm2 delete "$process_name" || true
    fi
    ;;
  "all")
    log "Removing all PM2 processes"
    sudo -u t8k pm2 kill || true

    # Remove PM2 startup service
    if command -v pm2 &>/dev/null; then
      pm2 unstartup systemd || true
    fi
    ;;
  esac
}

# Remove nginx configuration
remove_nginx_config() {
  local target="$1"
  local site_id="${2:-}"

  case "$target" in
  "main")
    log "Removing nginx configuration for main installation"
    rm -f /etc/nginx/sites-enabled/t8k-default.conf
    rm -f /etc/nginx/sites-available/t8k-default.conf
    ;;
  "site")
    log "Removing nginx configuration for site: $site_id"
    rm -f "/etc/nginx/sites-enabled/t8k-${site_id}.conf"
    rm -f "/etc/nginx/sites-available/t8k-${site_id}.conf"
    ;;
  "all")
    log "Removing all nginx configurations"
    rm -f /etc/nginx/sites-enabled/t8k-*.conf
    rm -f /etc/nginx/sites-available/t8k-*.conf
    ;;
  esac

  # Test nginx config and reload
  if nginx -t 2>/dev/null; then
    systemctl reload nginx
    log "Nginx configuration reloaded"
  else
    log_error "Nginx configuration test failed after cleanup"
  fi
}

# Update ports configuration
update_ports_config() {
  local target="$1"
  local site_id="${2:-}"

  if [[ ! -f "$PORTS_CONFIG_FILE" ]]; then
    log "Ports config file not found, skipping port cleanup"
    return 0
  fi

  case "$target" in
  "main")
    log "Removing main port allocation"
    local temp_file="${PORTS_CONFIG_FILE}.tmp"
    grep -v "^main=" "$PORTS_CONFIG_FILE" >"$temp_file" || true
    mv "$temp_file" "$PORTS_CONFIG_FILE"
    chown t8k:t8k "$PORTS_CONFIG_FILE" 2>/dev/null || true
    ;;
  "site")
    log "Removing port allocation for site: $site_id"
    local temp_file="${PORTS_CONFIG_FILE}.tmp"
    grep -v "^${site_id}=" "$PORTS_CONFIG_FILE" >"$temp_file" || true
    mv "$temp_file" "$PORTS_CONFIG_FILE"
    chown t8k:t8k "$PORTS_CONFIG_FILE" 2>/dev/null || true
    ;;
  "all")
    log "Removing ports configuration file"
    rm -f "$PORTS_CONFIG_FILE"
    return 0
    ;;
  esac

  # Regenerate PM2 ecosystem config if t8k user still exists
  if id "t8k" &>/dev/null && [[ -f "/home/t8k/etc/pm2/ecosystem.config.js" ]]; then
    log "Regenerating PM2 ecosystem configuration"
    sudo -u t8k pm2 startOrReload /home/t8k/etc/pm2/ecosystem.config.js || true
  fi
}

# Remove directories
remove_directories() {
  local target="$1"
  local site_id="${2:-}"

  case "$target" in
  "main")
    log "Removing main installation directories"
    rm -rf /home/t8k/src/
    rm -rf /home/t8k/bin/
    rm -rf /home/t8k/t8k-go-server/

    # Only remove state directory if no dedicated sites exist
    if [[ ! -d "/home/t8k/sites" ]] || [[ -z "$(ls -A /home/t8k/sites 2>/dev/null)" ]]; then
      rm -rf /home/t8k/state/
    fi
    ;;
  "site")
    log "Removing dedicated site directories: $site_id"
    rm -rf "/home/t8k/sites/${site_id}/"

    # Remove sites directory if empty
    if [[ -d "/home/t8k/sites" ]] && [[ -z "$(ls -A /home/t8k/sites 2>/dev/null)" ]]; then
      rmdir /home/t8k/sites/
    fi
    ;;
  "all")
    log "Removing all TractStack directories"
    rm -rf /home/t8k/
    ;;
  esac
}

# Check if any installations remain
check_remaining_installations() {
  if [[ ! -f "$PORTS_CONFIG_FILE" ]]; then
    return 1 # No config file means no installations
  fi

  if [[ ! -s "$PORTS_CONFIG_FILE" ]]; then
    return 1 # Empty config file means no installations
  fi

  return 0 # File exists and has content
}

# Remove build watcher (only if no installations remain)
remove_build_watcher_if_empty() {
  if ! check_remaining_installations; then
    log "No remaining installations, removing build watcher"

    if systemctl is-active --quiet t8k-build-watcher.path 2>/dev/null; then
      systemctl stop t8k-build-watcher.path
    fi
    if systemctl is-enabled --quiet t8k-build-watcher.path 2>/dev/null; then
      systemctl disable t8k-build-watcher.path
    fi
    if systemctl is-active --quiet t8k-build-watcher.service 2>/dev/null; then
      systemctl stop t8k-build-watcher.service
    fi
    if systemctl is-enabled --quiet t8k-build-watcher.service 2>/dev/null; then
      systemctl disable t8k-build-watcher.service
    fi

    rm -f /etc/systemd/system/t8k-build-watcher.*
    systemctl daemon-reload
  fi
}

# Remove t8k user (only for 'all' option)
remove_t8k_user() {
  if [[ "$UNINSTALL_TARGET" != "all" ]]; then
    return 0
  fi

  if id "t8k" &>/dev/null; then
    log "Removing t8k user and home directory"
    userdel -r t8k 2>/dev/null || {
      log_error "Failed to remove t8k user, attempting force removal"
      userdel -f t8k 2>/dev/null || true
    }
  else
    log "User t8k does not exist, skipping user removal"
  fi
}

# Validate target exists
validate_target() {
  # For 'all' target, always proceed (cleanup any artifacts)
  if [[ "$UNINSTALL_TARGET" == "all" ]]; then
    return 0
  fi

  local sites
  read -ra sites <<<"$(read_installed_sites)"

  case "$UNINSTALL_TARGET" in
  "main")
    local found=false
    for site in "${sites[@]}"; do
      if [[ "$site" == "main" ]]; then
        found=true
        break
      fi
    done
    if [[ "$found" == false ]]; then
      echo -e "${RED}Main installation not found${RESET}"
      exit 1
    fi
    ;;
  "site")
    local found=false
    for site in "${sites[@]}"; do
      if [[ "$site" == "$SITE_ID" ]]; then
        found=true
        break
      fi
    done
    if [[ "$found" == false ]]; then
      echo -e "${RED}Site '$SITE_ID' not found${RESET}"
      exit 1
    fi
    ;;
  esac
}

# Main uninstall function
perform_uninstall() {
  case "$UNINSTALL_TARGET" in
  "main")
    log "Starting main installation removal"
    remove_pm2_processes "main"
    update_ports_config "main"
    remove_systemd_services "main"
    remove_nginx_config "main"
    remove_directories "main"
    remove_build_watcher_if_empty
    echo -e "${GREEN}Main installation removed successfully${RESET}"
    ;;
  "site")
    log "Starting dedicated site removal: $SITE_ID"
    remove_pm2_processes "site" "$SITE_ID"
    update_ports_config "site" "$SITE_ID"
    remove_systemd_services "site" "$SITE_ID"
    remove_nginx_config "site" "$SITE_ID"
    remove_directories "site" "$SITE_ID"
    remove_build_watcher_if_empty
    echo -e "${GREEN}Site '$SITE_ID' removed successfully${RESET}"
    ;;
  "all")
    log "Starting complete removal of all TractStack installations"
    remove_pm2_processes "all"
    update_ports_config "all"
    remove_systemd_services "all"
    remove_nginx_config "all"
    remove_directories "all"
    remove_t8k_user
    echo -e "${GREEN}All TractStack installations removed successfully${RESET}"
    ;;
  esac
}

# Main execution
main() {
  if [[ $# -eq 0 ]]; then
    # Interactive mode
    show_header
    check_root
    choose_uninstall_target
    confirm_uninstall
    validate_target
    perform_uninstall
  else
    # CLI mode
    parse_args "$@"

    if [[ "$NON_INTERACTIVE" == false ]]; then
      show_header
    fi

    check_root

    if [[ -z "$UNINSTALL_TARGET" ]]; then
      echo -e "${RED}No uninstall target specified${RESET}"
      show_usage
      exit 1
    fi

    validate_target
    confirm_uninstall
    perform_uninstall
  fi

  log "TractStack uninstall completed"
}

# Run main function
main "$@"
