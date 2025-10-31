# fsbhoa_light

FSBHOA lighting controller

Using a PLC controller and a set of relays that control lighting around the Lodge.
User interface allows definition of Zones which map to PLC channels which drive the relays.
Each zone can be configured with a schedule for time periods per day of the week.
It can also be driven by a photo cell.

# Installation and Setup

This plugin requires a background Go service to communicate with the PLC hardware. Follow these steps to set it up after installing the WordPress plugin.

## Prerequisites

1.  **Server Access:** You need SSH access to the server running WordPress with `sudo` privileges.
2.  **Dependencies:** Ensure Go, Git, and MySQL/MariaDB client libraries are installed. The main `install.sh` script (if used) typically handles this.
3.  **Repository Cloned:** Clone this repository to the server (e.g., `/home/your_user/fsbhoa_light`).
4.  **Plugin Activated:** Activate the "FSBHOA Lighting Control" plugin in WordPress. This creates the necessary database tables.
5.  **Configuration:** Configure the Go Service Port, PLC Addresses, and API Key in the WordPress Admin (**Settings > Lighting Control**). Generate and save the API Key.

---

## Go Service Setup

1.  **Build the Go Service:**
    Navigate to the service directory and run the build script. Replace `your_user` with the actual username.
    ```bash
    cd /home/your_user/fsbhoa_light/lighting-service
    ./build.sh
    ```

2.  **Create the systemd Service File:**
    Create a new file to define the service using a text editor like `nano` or `vi`:
    ```bash
    sudo vi /etc/systemd/system/fsbhoa-lighting.service
    ```
    Paste the following content into the file. **Carefully verify and update** the `User`, `Group`, `WorkingDirectory`, and `ExecStart` paths to match your specific server setup and username.
    ```ini
    [Unit]
    Description=FSBHOA Lighting Control Service
    After=network.target mysql.service mariadb.service

    [Service]
    Type=simple
    User=your_user          # <-- IMPORTANT: Change to the user owning the files
    Group=your_user         # <-- IMPORTANT: Change to the user's group
    WorkingDirectory=/home/your_user/fsbhoa_light/lighting-service # <-- VERIFY PATH
    ExecStart=/home/your_user/fsbhoa_light/lighting-service/lighting-service # <-- VERIFY PATH
    Restart=always
    RestartSec=10
    StandardOutput=journal # Send logs to systemd journal
    StandardError=journal  # Send errors to systemd journal

    [Install]
    WantedBy=multi-user.target
    ```
    Note: be sure to remove the #comments or it will not work.
    Save and close the file.

3.  **Reload systemd:**
    Tell `systemd` to recognize the new service file.
    ```bash
    sudo systemctl daemon-reload
    ```

4.  **Enable and Start the Service:**
    Enable the service to start automatically on boot and start it now.
    ```bash
    sudo systemctl enable --now fsbhoa-lighting.service
    ```

5.  **Verify Service Status:**
    Check that the service started correctly.
    ```bash
    sudo systemctl status fsbhoa-lighting.service
    ```
    You should see `Active: active (running)` in green. If it failed, check the logs using:
    ```bash
    sudo journalctl -u fsbhoa-lighting.service -n 50 --no-pager
    ```

6.  **Configure Firewall (iptables):**
    Ensure the Go service's listening port (default `8085`) is allowed through the firewall.
    ```bash
    sudo iptables -A INPUT -p tcp --dport 8085 -j ACCEPT
    sudo netfilter-persistent save
    ```

7.  **Configure `sudoers` (for Restart Button):** ⚠️
    For the "Restart Lighting Service" button on the WordPress settings page to function, the web server user (usually `www-data`) needs permission to run specific `systemctl` commands via `sudo` without a password.
    Create and edit a new sudoers file using `visudo`:
    ```bash
    sudo visudo -f /etc/sudoers.d/www-data-lighting
    ```
    Add the following lines **exactly** as shown:
    ```
    # Allow www-data to restart and check status of the lighting service ONLY
    www-data ALL=(ALL) NOPASSWD: /bin/systemctl restart fsbhoa-lighting.service
    www-data ALL=(ALL) NOPASSWD: /bin/systemctl status fsbhoa-lighting.service
    www-data ALL=(ALL) NOPASSWD: /bin/systemctl start fsbhoa-lighting.service
    www-data ALL=(ALL) NOPASSWD: /bin/systemctl stop fsbhoa-lighting.service
    ```
    Save and exit the editor. **Security Note:** This configuration grants the web server permission *only* for managing this specific service.

---
