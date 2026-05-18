const input = document.getElementById('artist');
const button = document.getElementById('submit');
const result = document.getElementById('result');

input.addEventListener('input', () => {
  button.disabled = input.value.trim() === '';
});

button.addEventListener('click', async () => {
  result.textContent = '';
  button.disabled = true;
  try {
    const resp = await fetch('/setlistjob', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ artist: input.value.trim() }),
    });
    const data = await resp.json();
    if (!resp.ok) {
      result.textContent = 'Error: ' + (data.error || resp.statusText);
    } else {
      const link = document.createElement('a');
      link.href = data.playlist_url;
      link.target = '_blank';
      link.textContent = data.playlist_url;
      result.appendChild(link);
    }
  } catch (err) {
    result.textContent = 'Error: ' + err.message;
  } finally {
    button.disabled = input.value.trim() === '';
  }
});
