require('dotenv').config();
const express = require('express');
const { fetchEvents } = require('./services/eventFetcher');

const app = express();
const PORT = process.env.PORT || 3000;

// In-memory event cache
let cachedEvents = [];
let lastFetchTime = null;

app.set('view engine', 'ejs');
app.set('views', './views');

// Refresh events from Ticketmaster
async function refreshEvents() {
  try {
    cachedEvents = await fetchEvents();
    lastFetchTime = new Date();
    console.log(`[${lastFetchTime.toISOString()}] Events refreshed: ${cachedEvents.length} events cached.`);
  } catch (err) {
    console.error('Failed to refresh events:', err.message);
  }
}

// Routes
app.get('/', async (req, res) => {
  if (!lastFetchTime) {
    try {
      await refreshEvents();
    } catch (err) {
      return res.status(500).render('index', {
        events: [],
        lastFetchTime: 'N/A',
        totalEvents: 0
      });
    }
  }
  res.render('index', {
    events: cachedEvents,
    lastFetchTime: lastFetchTime ? lastFetchTime.toLocaleString() : 'N/A',
    totalEvents: cachedEvents.length
  });
});

app.get('/health', (req, res) => {
  res.status(200).json({ status: 'ok', uptime: process.uptime() });
});

app.get('/api/events', (req, res) => {
  res.json({
    total: cachedEvents.length,
    lastFetchTime: lastFetchTime,
    events: cachedEvents
  });
});

// Initial fetch and periodic refresh every 30 minutes
refreshEvents();
setInterval(refreshEvents, 30 * 60 * 1000);

app.listen(PORT, () => {
  console.log(`UniEvent server running on port ${PORT}`);
});
