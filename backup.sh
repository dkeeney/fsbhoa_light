#!/bin/bash

# =================================================================
# WordPress Backup Script
#
# This script creates a compressed backup of the WordPress database
# and the media library. It is designed to be run automatically
# via cron and relies on a ~/.my.cnf file for secure credentials.
# =================================================================

# --- BEGIN CONFIGURATION ---

# Your WordPress database name
DB_NAME="fsbhoa_light"

# The full path to your WordPress installation's root directory
# Example: /var/www/html
WP_PATH="/var/www/html"

# The directory where you want to store your backups.
# Using $HOME is the correct way to reference your home directory.
BACKUP_DIR="$HOME/backup_storage"

# --- END CONFIGURATION ---


# --- SCRIPT LOGIC (Do not edit below this line) ---

# Create a timestamp for the backup files (e.g., 2025-08-15)
TIMESTAMP=$(date +"%F")

# Check if backup directory exists, create if it doesn't
if [ ! -d "$BACKUP_DIR" ]; then
  echo "Backup directory $BACKUP_DIR does not exist. Creating it..."
  mkdir -p "$BACKUP_DIR"
fi

# Define backup file paths
DB_BACKUP_FILE="$BACKUP_DIR/wp_database_${TIMESTAMP}.sql.gz"
UPLOADS_BACKUP_FILE="$BACKUP_DIR/wp_uploads_${TIMESTAMP}.tar.gz"

echo "Starting WordPress backup process..."
echo "-----------------------------------"

# 1. Back up the Database
echo "Backing up database: $DB_NAME..."
# This command securely gets credentials from ~/.my.cnf,
# skips tablespaces, and pipes the output to gzip for compression.
mysqldump --no-tablespaces "$DB_NAME" | gzip > "$DB_BACKUP_FILE"

# This correctly checks the exit status of mysqldump (the first command
# in the pipe) to ensure the backup was actually successful.
if [ ${PIPESTATUS[0]} -ne 0 ]; then
  echo "ERROR: Database backup failed. mysqldump exited with an error."
  # Remove the failed (and likely empty) backup file
  rm "$DB_BACKUP_FILE" 2>/dev/null
  exit 1
else
  echo "Database backup complete: $DB_BACKUP_FILE"
fi

echo "-----------------------------------"

# 2. Back up the Media Library (uploads directory)
echo "Backing up media library (wp-content/uploads)..."
tar -czf "$UPLOADS_BACKUP_FILE" -C "$WP_PATH/wp-content/" "uploads"

# Check if tar was successful
if [ $? -ne 0 ]; then
  echo "ERROR: Media library backup failed."
  exit 1
else
  echo "Media library backup complete: $UPLOADS_BACKUP_FILE"
fi

echo "-----------------------------------"
echo "WordPress backup process finished successfully."
echo ""

# Optional: Clean up old backups (older than 30 days)
echo "Cleaning up backups older than 30 days..."
find "$BACKUP_DIR" -type f -name "*.gz" -mtime +30 -exec rm {} \;
echo "Cleanup complete."

exit 0


