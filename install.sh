#!/bin/bash

# ==============================================================================
# FSBHOA Lighting Control - Full Server Installation Script
# This script sets up a fresh Raspberry Pi OS / Debian system.
# ==============================================================================

# --- Configuration (EDIT THESE VARIABLES) ---
# The user who will own and run the application files (e.g., 'pi')
APP_USER="pi"
# The URL to your GitHub repository
REPO_URL="https://github.com/dkeeney/fsbhoa_light.git"

# --- Safety Check: Must be run with sudo ---
if [ "$EUID" -ne 0 ]; then
  echo "Please run this script with sudo."
  exit 1
fi

echo "--- Starting FSBHOA Lighting Control Server Installation ---"

# --- 1. System Dependencies ---
echo "[1/7] Installing system packages..."
apt-get update
apt-get install -y \
    apache2 \
    mariadb-server \
    php libapache2-mod-php php-mysql php-json php-imagick php-gd php-intl \
    golang-go \
    git \
    wget \
    ufw \
    phpmyadmin || { echo "Failed to install packages."; exit 1; }

# --- 2. Configure Firewall (UFW) ---
echo "[2/7] Configuring firewall..."
# Allow essential services
ufw allow ssh
ufw allow 'Apache Full'

# Allow Go service port for status monitor
ufw allow 8085/tcp

# Enable the firewall
ufw --force enable
echo "Firewall enabled and configured."

# --- 3. Clone Project Repository ---
echo "[3/7] Cloning project repository from GitHub..."
USER_HOME=$(getent passwd "$APP_USER" | cut -d: -f6)
PROJECT_DIR="$USER_HOME/fsbhoa_lighting"

if [ ! -d "$PROJECT_DIR" ]; then
    # Run git clone as the specified application user
    sudo -u "$APP_USER" git clone "$REPO_URL" "$PROJECT_DIR"
    echo "Repository cloned successfully."
else
    echo "Project directory already exists. Skipping clone."
fi

# --- 4. Download and Prepare WordPress ---
echo "[4/7] Setting up WordPress..."
if [ ! -f /var/www/html/wp-config-sample.php ]; then
    wget https://wordpress.org/latest.tar.gz -O /tmp/latest.tar.gz
    tar -xzf /tmp/latest.tar.gz -C /tmp
    cp -r /tmp/wordpress/* /var/www/html/
    rm -f /var/www/html/index.html # Remove default Apache page
    chown -R www-data:www-data /var/www/html/
    find /var/www/html/ -type d -exec chmod 755 {} \;
    find /var/www/html/ -type f -exec chmod 644 {} \;
else
    echo "WordPress already appears to be installed. Skipping download."
fi

# --- 5. Link Plugin to WordPress ---
echo "[5/7] Linking plugin to WordPress directory..."
PLUGIN_TARGET="/var/www/html/wp-content/plugins/fsbhoa_lighting"
if [ ! -L "$PLUGIN_TARGET" ]; then
    # Remove directory if it exists, then create link
    rm -rf "$PLUGIN_TARGET"
    ln -s "$PROJECT_DIR" "$PLUGIN_TARGET"
    echo "Symbolic link created."
else
    echo "Symbolic link already exists. Skipping."
fi

# --- 6. Create systemd Service File ---
echo "[6/7] Creating systemd service file for the Go service..."
LIGHTING_SERVICE_DIR="$PROJECT_DIR/lighting-service"

cat << EOF > /etc/systemd/system/fsbhoa-lighting.service
[Unit]
Description=FSBHOA Lighting Control Service
After=network.target mysql.service

[Service]
Type=simple
User=$APP_USER
Group=$(id -gn "$APP_USER")
WorkingDirectory=$LIGHTING_SERVICE_DIR
ExecStart=$LIGHTING_SERVICE_DIR/lighting-service
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
echo "systemd service file created."

# --- 7. Final Apache Configuration ---
echo "[7/7] Configuring Apache..."
a2enmod rewrite
# Ensure phpmyadmin config is enabled
a2enconf phpmyadmin.conf
systemctl restart apache2
echo "Apache configured and restarted."


# --- Final Instructions ---
echo ""
echo "--- Automated Setup Complete ---"
echo ""
echo "Next Steps (Manual Configuration Required):"
echo "1. Secure the MariaDB installation: sudo mysql_secure_installation"
echo "2. Create a database ('fsbhoa_db') and user ('wp_user') for WordPress."
echo "3. Complete the WordPress installation via your web browser at http://<your-pi-ip>/"
echo "4. Activate the 'FSBHOA Lighting Control' plugin in the WordPress dashboard."
echo "5. Build the Go service:"
echo "   cd $LIGHTING_SERVICE_DIR"
echo "   ./build.sh"
echo "6. Enable and start the Go service:"
echo "   sudo systemctl enable --now fsbhoa-lighting.service"
echo ""

