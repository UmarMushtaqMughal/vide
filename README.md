# UniEvent — University Event Management System

A Node.js/Express web application that fetches live events from the **Ticketmaster Discovery API** and displays them as university events. Built for CE 308/408 Cloud Computing at GIKI and deployed on AWS.

---

## Architecture Overview

```
Internet ──► ALB (Application Load Balancer)
                │
                ▼
         EC2 Instance (Amazon Linux 2023)
         ┌──────────────────────────────┐
         │  Node.js 18 + Express        │
         │  EJS templating              │
         │  Ticketmaster API client     │
         └──────────────────────────────┘
                │
                ▼
         AWS Services
         ├── VPC  (network isolation)
         ├── EC2  (compute)
         ├── S3   (media/static assets)
         ├── ALB  (load balancing + health checks)
         └── IAM  (role-based access control)
```

---

## Features

- 🎫 **Live Events** — Fetches 20 events from Ticketmaster every 30 minutes
- 🖼️ **Rich Cards** — Best-quality 16:9 images, category badges, price ranges
- ⚡ **In-Memory Cache** — Fast response times without hitting the API on every request
- 🏥 **Health Endpoint** — `/health` route for ALB health checks
- 🔌 **JSON API** — `/api/events` for programmatic access
- ☁️ **AWS-Ready** — EC2 bootstrap script (`user-data.sh`) included

---

## Quick Start (Local)

### Prerequisites

- Node.js 18 or higher
- A free [Ticketmaster Developer API key](https://developer.ticketmaster.com/)

### Setup

```bash
# 1. Clone the repository
git clone https://github.com/UmarMushtaqMughal/vide.git
cd vide

# 2. Install dependencies
npm install

# 3. Configure environment variables
cp .env.example .env
# Edit .env and set your TICKETMASTER_API_KEY

# 4. Start the server
npm start
# Open http://localhost:3000
```

---

## AWS Deployment

### EC2 Launch (User Data)

Use `user-data.sh` as the **User Data** script when launching an EC2 instance. It will:

1. Install Node.js 18 on Amazon Linux 2023
2. Create the app directory at `/home/ec2-user/unievent`
3. Install dependencies
4. Start the server on port 3000

### ALB Health Check

Configure your Application Load Balancer health check to hit:

```
GET /health
```

Expected response: `HTTP 200` with JSON body `{"status":"ok","uptime":<seconds>}`

---

## API Reference

### `GET /`
Renders the main event listing page (HTML).

### `GET /health`
Returns server health status (used by ALB).

```json
{ "status": "ok", "uptime": 123.45 }
```

### `GET /api/events`
Returns all cached events as JSON.

```json
{
  "total": 20,
  "lastFetchTime": "2024-01-01T00:00:00.000Z",
  "events": [
    {
      "id": "...",
      "title": "Event Name",
      "date": "2024-06-15",
      "time": "19:00:00",
      "venue": "Venue Name",
      "description": "...",
      "imageUrl": "https://...",
      "category": "Music",
      "priceRange": "$20 - $80",
      "url": "https://www.ticketmaster.com/..."
    }
  ]
}
```

---

## Environment Variables

| Variable | Description | Default |
|---|---|---|
| `TICKETMASTER_API_KEY` | Ticketmaster Discovery API key | *(required)* |
| `PORT` | Port the server listens on | `3000` |
| `AWS_REGION` | AWS region for SDK calls | `us-east-1` |
| `S3_BUCKET_NAME` | S3 bucket for media assets | `unievent-media-bucket` |

Copy `.env.example` to `.env` and fill in the values.

---

## Folder Structure

```
vide/
├── server.js               # Main application entry point
├── package.json            # Node.js dependencies & scripts
├── .env.example            # Environment variable template
├── .gitignore              # Git ignore rules
├── user-data.sh            # EC2 bootstrap script
├── services/
│   └── eventFetcher.js     # Ticketmaster API integration
├── views/
│   └── index.ejs           # EJS frontend template
└── README.md               # This file
```

---

## Course Information

| Field | Details |
|---|---|
| Course | CE 308/408 Cloud Computing |
| Institution | GIKI (Ghulam Ishaq Khan Institute) |
| Project | UniEvent — University Event Management System |
| Cloud Provider | Amazon Web Services (AWS) |

---

## License

This project is open source. Please check the repository for license details.

## Author

UmarMushtaqMughal

## Links

- GitHub: https://github.com/UmarMushtaqMughal/vide
- Ticketmaster Developer: https://developer.ticketmaster.com/
