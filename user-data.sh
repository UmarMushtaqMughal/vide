#!/bin/bash
# user-data.sh — EC2 Bootstrap Script for UniEvent on Amazon Linux 2023
set -e

# Update system packages
dnf update -y

# Install Node.js 18 via NodeSource
curl -fsSL https://rpm.nodesource.com/setup_18.x | bash -
dnf install -y nodejs git

# Verify installation
node -v
npm -v

# Clone the application repository
git clone https://github.com/UmarMushtaqMughal/vide.git /home/ec2-user/unievent
cd /home/ec2-user/unievent

# Create .env from template and inject the API key from EC2 instance metadata / SSM / manual step
# Replace the placeholder below with your actual Ticketmaster API key,
# or pass it via EC2 User Data environment variables or AWS SSM Parameter Store.
cp .env.example .env
# sed -i "s/your_api_key_here/$TICKETMASTER_API_KEY/" .env

# Install Node.js dependencies
npm install --production

# Start the server using PM2 process manager
npm install -g pm2
pm2 start server.js --name unievent
pm2 startup systemd -u ec2-user --hp /home/ec2-user
pm2 save

echo "UniEvent started successfully on port 3000."
