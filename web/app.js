'use strict';

const grid = document.getElementById('image-grid');
const emptyMsg = document.getElementById('empty-msg');
const uploadForm = document.getElementById('upload-form');
const fileInput = document.getElementById('file-input');
const fileLabelText = document.getElementById('file-label-text');
const uploadBtn = document.getElementById('upload-btn');
const uploadError = document.getElementById('upload-error');

/** @type {Map<string, number>} imageId → intervalId for active polls */
const polls = new Map();

// ── Bootstrap ────────────────────────────────────────────────────────────────

loadImages();

fileInput.addEventListener('change', () => {
  fileLabelText.textContent = fileInput.files[0]
    ? fileInput.files[0].name
    : 'Choose a JPEG or PNG file…';
});

uploadForm.addEventListener('submit', async (e) => {
  e.preventDefault();
  uploadError.classList.add('hidden');

  const file = fileInput.files[0];
  if (!file) return;

  uploadBtn.disabled = true;
  uploadBtn.textContent = 'Uploading…';

  try {
    const fd = new FormData();
    fd.append('file', file);

    const res = await fetch('/upload', { method: 'POST', body: fd });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.error || `HTTP ${res.status}`);
    }

    const img = await res.json();
    addCard(img);
    startPoll(img.id);

    fileInput.value = '';
    fileLabelText.textContent = 'Choose a JPEG or PNG file…';
  } catch (err) {
    uploadError.textContent = `Upload failed: ${err.message}`;
    uploadError.classList.remove('hidden');
  } finally {
    uploadBtn.disabled = false;
    uploadBtn.textContent = 'Upload & Process';
  }
});

// ── Load existing images ──────────────────────────────────────────────────────

async function loadImages() {
  try {
    const res = await fetch('/images?limit=100&offset=0');
    if (!res.ok) return;

    const items = await res.json();
    if (!Array.isArray(items) || items.length === 0) {
      emptyMsg.classList.remove('hidden');
      return;
    }

    for (const img of items) {
      addCard(img);
      if (img.status === 'pending' || img.status === 'processing') {
        startPoll(img.id);
      }
    }
  } catch (_) { /* network error on load — ignore */ }
}

// ── Card rendering ────────────────────────────────────────────────────────────

function addCard(img) {
  emptyMsg.classList.add('hidden');

  const card = document.createElement('div');
  card.className = 'card';
  card.id = `card-${img.id}`;
  card.innerHTML = cardHTML(img);

  card.querySelector('.btn-delete').addEventListener('click', () => deleteImage(img.id));
  grid.prepend(card);
}

function updateCard(img) {
  const card = document.getElementById(`card-${img.id}`);
  if (!card) return;
  card.innerHTML = cardHTML(img);
  card.querySelector('.btn-delete').addEventListener('click', () => deleteImage(img.id));
}

function cardHTML(img) {
  const badgeClass = `badge-${img.status}`;
  const preview = img.status === 'done'
    ? `<img src="/image/${img.id}/file?variant=thumbnail" alt="${esc(img.original_name)}" loading="lazy" />`
    : `<div class="placeholder">${statusLabel(img.status)}</div>`;

  const variants = (img.variants || []).map(v =>
    `<a href="/image/${img.id}/file?variant=${v.type}" download>${v.type}</a>`
  ).join('');

  return `
    <div class="card-preview">${preview}</div>
    <div class="card-body">
      <div class="card-name" title="${esc(img.original_name)}">${esc(img.original_name)}</div>
      <span class="badge ${badgeClass}">${esc(img.status)}</span>
      ${variants ? `<div class="card-variants">${variants}</div>` : ''}
    </div>
    <div class="card-footer">
      <button class="btn-delete">Delete</button>
    </div>`;
}

function statusLabel(status) {
  const labels = {
    pending: '⏳ Waiting…',
    processing: '⚙️ Processing…',
    done: '✅ Done',
    failed: '❌ Failed',
    cancelled: '🚫 Cancelled',
  };
  return labels[status] || status;
}

function esc(str) {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

// ── Polling ───────────────────────────────────────────────────────────────────

function startPoll(id) {
  if (polls.has(id)) return;

  const intervalId = setInterval(async () => {
    try {
      const res = await fetch(`/image/${id}`);
      if (!res.ok) { stopPoll(id); return; }

      const img = await res.json();
      updateCard(img);

      if (img.status !== 'pending' && img.status !== 'processing') {
        stopPoll(id);
      }
    } catch (_) { stopPoll(id); }
  }, 2000);

  polls.set(id, intervalId);
}

function stopPoll(id) {
  const intervalId = polls.get(id);
  if (intervalId !== undefined) {
    clearInterval(intervalId);
    polls.delete(id);
  }
}

// ── Delete ────────────────────────────────────────────────────────────────────

async function deleteImage(id) {
  stopPoll(id);

  try {
    const res = await fetch(`/image/${id}`, { method: 'DELETE' });
    if (!res.ok && res.status !== 404) {
      alert('Delete failed. Please try again.');
      return;
    }
  } catch (_) {
    alert('Delete failed. Please try again.');
    return;
  }

  const card = document.getElementById(`card-${id}`);
  if (card) card.remove();

  if (grid.children.length === 0) {
    emptyMsg.classList.remove('hidden');
  }
}
