const clientId = '2b6d4218c01949b9ba9bdc8c0b1a7c97'; // committing my client ID for deploy simplicity, to avoid JS builds for now
const redirectUri = window.location.origin;
const scope = 'playlist-modify-public playlist-modify-private';

// --- PKCE helpers ---

const generateRandomString = (length) => {
  const possible = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
  const values = crypto.getRandomValues(new Uint8Array(length));
  return values.reduce((acc, x) => acc + possible[x % possible.length], '');
};

const sha256 = async (plain) => {
  const data = new TextEncoder().encode(plain);
  return window.crypto.subtle.digest('SHA-256', data);
};

const base64encode = (input) =>
  btoa(String.fromCharCode(...new Uint8Array(input)))
    .replace(/=/g, '')
    .replace(/\+/g, '-')
    .replace(/\//g, '_');

async function redirectToAuthCodeFlow() {
  const verifier = generateRandomString(64);
  const challenge = base64encode(await sha256(verifier));
  localStorage.setItem('code_verifier', verifier);

  const authUrl = new URL('https://accounts.spotify.com/authorize');
  authUrl.search = new URLSearchParams({
    response_type: 'code',
    client_id: clientId,
    scope: scope,
    code_challenge_method: 'S256',
    code_challenge: challenge,
    redirect_uri: redirectUri,
  }).toString();

  window.location.href = authUrl.toString();
}

async function getAccessToken(code) {
  const verifier = localStorage.getItem('code_verifier');
  const response = await fetch('https://accounts.spotify.com/api/token', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({
      client_id: clientId,
      grant_type: 'authorization_code',
      code: code,
      redirect_uri: redirectUri,
      code_verifier: verifier,
    }),
  });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error_description || 'Token exchange failed');
  localStorage.setItem('spotify_access_token', data.access_token);
  // Remove code from URL without adding a history entry
  window.history.replaceState({}, '', window.location.pathname);
}

// --- UI ---

const connectBtn = document.getElementById('connect');
const mainDiv = document.getElementById('main');
const input = document.getElementById('artist');
const submitBtn = document.getElementById('submit');
const result = document.getElementById('result');

function showMain() {
  connectBtn.style.display = 'none';
  mainDiv.style.display = '';
}

function showConnect() {
  connectBtn.style.display = '';
  mainDiv.style.display = 'none';
}

async function init() {
  const params = new URLSearchParams(window.location.search);
  const code = params.get('code');

  if (code) {
    try {
      await getAccessToken(code);
    } catch (err) {
      result.innerHTML = `<div class="notification is-danger">Spotify auth error: ${err.message}</div>`;
    }
  }

  if (localStorage.getItem('spotify_access_token')) {
    showMain();
  } else {
    showConnect();
  }
}

connectBtn.addEventListener('click', () => redirectToAuthCodeFlow());

let requestCompleted = false;

input.addEventListener('input', () => {
  if (requestCompleted) {
    requestCompleted = false;
    submitBtn.textContent = 'Create Playlist';
    result.innerHTML = '';
  }
  submitBtn.disabled = input.value.trim() === '';
});

submitBtn.addEventListener('click', async () => {
  result.innerHTML = '';
  submitBtn.disabled = true;
  submitBtn.classList.add('is-loading');
  try {
    const resp = await fetch('/setlistjob', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Autosetlist-Spotify-Token': localStorage.getItem('spotify_access_token')
      },
      body: JSON.stringify({ artist: input.value.trim() }),
    });
    const data = await resp.json();
    if (!resp.ok) {
      requestCompleted = true;
      result.innerHTML = `<div class="notification is-danger">${data.error || resp.statusText}</div>`;
    } else {
      requestCompleted = true;
      submitBtn.textContent = 'Created!';
      submitBtn.disabled = true;
      result.innerHTML = `<a href="${data.playlist_url}" class="button is-primary is-rounded" target="_blank">Listen on Spotify</a>`;
    }
  } catch (err) {
    requestCompleted = true;
    result.innerHTML = `<div class="notification is-danger">${err.message}</div>`;
  } finally {
    submitBtn.classList.remove('is-loading');
    if (submitBtn.textContent !== 'Created!') submitBtn.disabled = input.value.trim() === '';
  }
});

init();
