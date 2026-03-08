const axios = require('axios');

const TICKETMASTER_BASE_URL = 'https://app.ticketmaster.com/discovery/v2/events.json';

/**
 * Pick the best quality 16:9 image from Ticketmaster image array.
 */
function pickBestImage(images) {
  if (!images || images.length === 0) return '';
  const ratio169 = images.filter(img => img.ratio === '16_9');
  const pool = ratio169.length > 0 ? ratio169 : images;
  pool.sort((a, b) => (b.width || 0) - (a.width || 0));
  return pool[0].url || '';
}

/**
 * Transform a raw Ticketmaster event object into UniEvent format.
 */
function transformEvent(raw) {
  const venue =
    raw._embedded && raw._embedded.venues && raw._embedded.venues[0]
      ? raw._embedded.venues[0].name
      : 'TBD';

  const dateInfo = raw.dates && raw.dates.start ? raw.dates.start : {};
  const date = dateInfo.localDate || 'TBD';
  const time = dateInfo.localTime || 'TBD';

  const category =
    raw.classifications && raw.classifications[0]
      ? raw.classifications[0].segment
        ? raw.classifications[0].segment.name
        : 'General'
      : 'General';

  const priceRanges = raw.priceRanges && raw.priceRanges.length > 0 ? raw.priceRanges[0] : null;
  const priceRange = priceRanges
    ? `$${priceRanges.min} - $${priceRanges.max}`
    : 'Free / TBD';

  return {
    id: raw.id,
    title: raw.name,
    date,
    time,
    venue,
    description: raw.info || raw.pleaseNote || 'No description available.',
    imageUrl: pickBestImage(raw.images),
    category,
    priceRange,
    url: raw.url || '#'
  };
}

/**
 * Fetch 20 events from Ticketmaster Discovery API and return them in UniEvent format.
 */
async function fetchEvents() {
  const apiKey = process.env.TICKETMASTER_API_KEY;
  if (!apiKey) {
    throw new Error('TICKETMASTER_API_KEY is not set in environment variables.');
  }

  let response;
  try {
    response = await axios.get(TICKETMASTER_BASE_URL, {
      params: {
        apikey: apiKey,
        size: 20
      }
    });
  } catch (err) {
    const status = err.response ? err.response.status : 'N/A';
    const data = err.response ? JSON.stringify(err.response.data) : err.message;
    throw new Error(`Failed to fetch events from Ticketmaster API (status ${status}): ${data}`);
  }

  const rawEvents =
    response.data &&
    response.data._embedded &&
    response.data._embedded.events
      ? response.data._embedded.events
      : [];

  return rawEvents.map(transformEvent);
}

module.exports = { fetchEvents };
