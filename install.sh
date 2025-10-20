#!/bin/bash

# FSBHOA Access Control System - Server & Kiosk Installation Script
# This script is tailored for the access.fsbhoa.com dual-purpose machine.
# It should be run on a fresh Ubuntu Desktop LTS system.

# --- Safety Check: Must be run with sudo ---
if [ "$EUID" -ne 0 ]; then
  echo "Please run this script with sudo."
  exit 1
fi

echo "--- Starting FSBHOA Server & Kiosk Installation for access.fsbhoa.com ---"

# --- 1. System Dependencies ---
echo "[1/6] Installing system packages (Apache, MySQL, PHP, Go, Git)..."
apt-get update
apt-get install -y \
    apache2 \
    mysql-server \
    php libapache2-mod-php php-mysql \
    golang-go \
    git \
    iptables-persistent \
    wget || { echo "Failed to install packages."; exit 1; }

# --- 2. Download and Prepare WordPress ---
echo "[2/6] Downloading and setting up WordPress..."
if [ ! -f /var/www/html/index.php ]; then
    wget https://wordpress.org/latest.tar.gz -O /tmp/latest.tar.gz
    tar -xzf /tmp/latest.tar.gz -C /tmp
    cp -r /tmp/wordpress/* /var/www/html/
    rm -rf /var/www/html/index.html # Remove default Apache page
    chown -R www-data:www-data /var/www/html/
    find /var/www/html/ -type d -exec chmod 755 {} \;
    find /var/www/html/ -type f -exec chmod 644 {} \;
else
    echo "WordPress already appears to be installed. Skipping download."
fi

# --- 3. Create Application Configuration Directory ---
echo "[3/6] Creating configuration directory /var/lib/fsbhoa..."
mkdir -p /var/lib/fsbhoa
# Set ownership to www-data so the WordPress plugin can write config files
chown -R www-data:www-data /var/lib/fsbhoa
chmod -R 775 /var/lib/fsbhoa

# --- 4. Configure Firewall (iptables) ---
echo "[4/6] Configuring iptables firewall..."
# Flush existing rules
iptables -F

# Allow established connections
iptables -A INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

# Allow loopback
iptables -A INPUT -i lo -j ACCEPT

# Allow standard web and system ports
iptables -A INPUT -p tcp --dport 22 -j ACCEPT   # SSH
iptables -A INPUT -p tcp --dport 80 -j ACCEPT   # HTTP (for redirect to HTTPS)
iptables -A INPUT -p tcp --dport 443 -j ACCEPT  # HTTPS

# Allow ports for Go backend services
iptables -A INPUT -p tcp --dport 8081 -j ACCEPT  # print_service
iptables -A INPUT -p tcp --dport 8082 -j ACCEPT  # monitor_service (websocket)
iptables -A INPUT -p tcp --dport 8083 -j ACCEPT  # event_service
# The kiosk Go app will also run on this machine
iptables -A INPUT -p tcp --dport 8080 -j ACCEPT  # kiosk_service

# Allow UHPPOTE broadcast and event ports
iptables -A INPUT -p udp --dport 60000 -j ACCEPT # Discovery broadcast
iptables -A INPUT -p udp --dport 60001 -j ACCEPT # Controller reply port
iptables -A INPUT -p udp --dport 60002 -j ACCEPT # Event listener port

# Drop all other incoming traffic
iptables -P INPUT DROP

# Save the rules to make them persistent
netfilter-persistent save
echo "Firewall enabled and configured."

# --- 5. Create systemd Service Files for Go Apps ---
echo "[5/6] Creating systemd service files..."

# Get the username of the user who ran sudo
SUDO_USER_VAR=$(logname)
USER_HOME=$(getent passwd "$SUDO_USER_VAR" | cut -d: -f6)
PROJECT_DIR="$USER_HOME/fsbhoa_ac"

# Service for Event Handler
cat << EOF > /etc/systemd/system/fsbhoa-events.service
[Unit]
Description=FSBHOA Hardware Event Service
After=network.target

[Service]
Type=simple
User=$SUDO_USER_VAR
Group=$(id -gn "$SUDO_USER_VAR")
WorkingDirectory=$PROJECT_DIR/event_service
ExecStart=$PROJECT_DIR/event_service/event_service
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF


# Service for Real-time monitor service
cat << EOF > /etc/systemd/system/fsbhoa-monitor.service
[Unit]
Description=FSBHOA Monitor Service
After=network.target

[Service]
Type=simple
User=$SUDO_USER_VAR
Group=$(id -gn "$SUDO_USER_VAR")
WorkingDirectory=$PROJECT_DIR/monitor_service
ExecStart=$PROJECT_DIR/monitor_service/monitor_service
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF


# Service for Zebra Printer
cat << EOF > /etc/systemd/system/fsbhoa-zebra_printer.service
[Unit]
Description=FSBHOA Zebra Card Printer Service
After=network.target

[Service]
Type=simple
User=$SUDO_USER_VAR
Group=$(id -gn "$SUDO_USER_VAR")
WorkingDirectory=$PROJECT_DIR/zebra_print_service
ExecStart=$PROJECT_DIR/zebra_print_service/zebra_print_service
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Service for Kiosk App (to run on this machine)
cat << EOF > /etc/systemd/system/fsbhoa-kiosk.service
[Unit]
Description=FSBHOA Resident Sign-in Kiosk App
After=network.target

[Service]
Type=simple
User=$SUDO_USER_VAR
Group=$(id -gn "$SUDO_USER_VAR")
WorkingDirectory=$PROJECT_DIR/kiosk
ExecStart=$PROJECT_DIR/kiosk/kiosk
StandardInput=file:/dev/input/by-id/usb-YOUR_CARD_READER_ID-event-kbd
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo apt install phpmyadmin
sudo apt install php-json php-imagick php-gd php-intl
sudo systemctl restart apache2
#sudo a2enmod rewrite

systemctl daemon-reload
echo "systemd service files created."

# --- 6. Final Instructions ---
echo ""
echo "--- Automated Setup Complete ---"
echo ""
echo "Next Steps (Manual Configuration Required):"
echo "1. Secure the MySQL installation: sudo mysql_secure_installation"
echo "2. Create a database and user for WordPress."
echo "3. Configure Apache and SSL for access.fsbhoa.com (see INSTALL.md)."
echo "4. Complete the WordPress installation via your web browser."
echo "5. Build each Go application and the uhppote-cli tool."
echo "6. Enable and start the Go services using systemctl."
echo ""
echo "Refer to INSTALL.md for detailed instructions."

