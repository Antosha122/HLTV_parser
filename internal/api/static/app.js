const $ = (id) => document.getElementById(id);

function asArray(v) {
  return Array.isArray(v) ? v : [];
}

async function api(path, opts = {}) {
  let res;
  try {
    res = await fetch(path, {
      headers: { 'Content-Type': 'application/json' },
      ...opts,
    });
  } catch (_) {
    throw new Error('Сервер не отвечает — запустите run.bat и откройте http://127.0.0.1:8080');
  }
  let data = {};
  try {
    data = await res.json();
  } catch (_) {}
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

const eventStatusRu = {
  ongoing: 'идёт',
  upcoming: 'предстоящий',
  past: 'завершён',
  over: 'завершён',
};

const phaseLabels = {
  start: 'Старт',
  events: 'Турниры',
  events_list: 'Турниры',
  event: 'Матчи турнира',
  history: 'История команд',
  teams: 'Все команды',
  maps: 'Статистика карт',
  predict: 'Подготовка прогноза',
  team: 'Загрузка команды',
  done: 'Завершено',
  error: 'Ошибка',
};

const cancellablePhases = new Set(['start', 'events', 'event', 'history', 'teams', 'maps']);

const breakdownRu = {
  elo: 'Рейтинг Elo',
  form: 'Форма',
  h2h: 'Личные встречи',
  maps: 'Карты',
  final: 'Итог',
};

const vetoActionRu = {
  ban: 'бан',
  pick: 'выбор',
  leftover: 'десайдер',
  remove: 'удаление',
};

const confidenceRu = {
  high: 'высокая',
  medium: 'средняя',
  low: 'низкая',
};

let syncPollTimer = null;
let refreshPending = false;
let wasSyncRunning = false;

function showStopRefresh(show) {
  const btn = $('stopRefreshBtn');
  if (!btn) return;
  if (show) {
    btn.classList.remove('hidden');
    btn.disabled = false;
  } else {
    btn.classList.add('hidden');
    btn.disabled = true;
  }
}

function showSyncProgress(show) {
  const panel = $('syncProgress');
  if (!panel) return;
  if (show) {
    panel.classList.remove('hidden');
    panel.classList.add('active');
  }
}
let eventTeamsCache = [];
let eventSearchTimer = null;
let teamSearchTimer = null;
let profileSearchTimer = null;
let lastDbSnapshot = '';

function debounce(fn, ms) {
  let t;
  return (...args) => {
    clearTimeout(t);
    t = setTimeout(() => fn(...args), ms);
  };
}

function initTabs() {
  document.querySelectorAll('.tab').forEach((btn) => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('.tab').forEach((b) => b.classList.remove('active'));
      document.querySelectorAll('.tab-panel').forEach((p) => p.classList.remove('active'));
      btn.classList.add('active');
      const panel = $(`tab-${btn.dataset.tab}`);
      if (panel) panel.classList.add('active');
      if (btn.dataset.tab === 'team' || btn.dataset.tab === 'predict') {
        reloadAllLists().catch((err) => {
          if (btn.dataset.tab === 'team') {
            $('profileStatus').textContent = err.message || 'Ошибка загрузки списков';
          }
        });
      }
      if (btn.dataset.tab === 'team') {
        loadProfileTeamList($('profileTeamSearch')?.value?.trim() || '').catch((err) => {
          $('profileStatus').textContent = err.message || 'Не удалось загрузить команды';
        });
      }
    });
  });
}

function ensureSyncPolling() {
  if (!syncPollTimer) {
    syncPollTimer = setInterval(pollSyncStatus, 1500);
  }
}

async function waitForOperation() {
  ensureSyncPolling();
  for (;;) {
    const st = await api('/api/sync/status');
    renderSyncStatus(st);
    if (!st.running) {
      if (st.phase === 'error') {
        throw new Error(st.detail || 'Операция не удалась');
      }
      return st;
    }
    await new Promise((r) => setTimeout(r, 1500));
  }
}

async function reloadAllLists() {
  const [eventsRes, teamsRes] = await Promise.allSettled([
    loadEvents($('eventSearch').value.trim()),
    loadProfileTeamList($('profileTeamSearch').value.trim()),
  ]);
  if (eventsRes.status === 'rejected') {
    $('status').textContent = `Турниры: ${eventsRes.reason?.message || 'ошибка загрузки'}`;
  }
  if (teamsRes.status === 'rejected') {
    $('profileStatus').textContent = `Команды: ${teamsRes.reason?.message || 'ошибка загрузки'}`;
  }

  const eventId = $('eventSelect').value;
  if (eventId) {
    await loadEventTeams(eventId);
  } else {
    await loadAllTeamsForPredict();
  }
}

async function loadAllTeamsForPredict() {
  const teams = asArray(await api('/api/teams?limit=100'));
  eventTeamsCache = teams;
  $('teamSearch').disabled = false;
  applyTeamFilter();
  if (!teams.length) {
    $('status').textContent = 'Нет команд в БД — нажмите «Обновить турниры и команды»';
  } else if (!$('eventSelect').value) {
    $('status').textContent = `Все команды из БД: ${teams.length} (или выберите турнир)`;
  }
}

function bindClick(id, handler) {
  const el = $(id);
  if (el) el.addEventListener('click', handler);
}

function fillSelect(select, items, emptyText) {
  if (!select) return;
  const prev = select.value;
  select.innerHTML = '';
  if (!items.length) {
    const opt = document.createElement('option');
    opt.value = '';
    opt.textContent = emptyText || '— нет данных —';
    select.appendChild(opt);
    return;
  }
  for (const item of items) {
    const opt = document.createElement('option');
    opt.value = item.id;
    const rank = item.world_rank ? ` #${item.world_rank}` : '';
    const matches = item.matches_count != null ? ` · ${item.matches_count} матч.` : '';
    opt.textContent = `${item.name}${rank}${matches}${teamUpdatedLabel(item)}`;
    select.appendChild(opt);
  }
  if (prev && [...select.options].some((o) => o.value === prev)) {
    select.value = prev;
  }
}

function fillEvents(events) {
  const sel = $('eventSelect');
  const prev = sel.value;
  sel.innerHTML = '<option value="">— все команды из БД —</option>';
  for (const e of events) {
    const opt = document.createElement('option');
    opt.value = e.id;
    const st = e.status ? eventStatusRu[e.status] || e.status : '';
    const tag = st ? ` [${st}]` : '';
    opt.textContent = `${e.name}${tag}`;
    sel.appendChild(opt);
  }
  if (prev) sel.value = prev;
}

function formatTime(iso) {
  if (!iso) return '';
  try {
    return new Date(iso).toLocaleString('ru-RU');
  } catch (_) {
    return iso;
  }
}

function formatDateShort(iso) {
  if (!iso) return '';
  try {
    return new Date(iso).toLocaleDateString('ru-RU');
  } catch (_) {
    return iso.slice(0, 10);
  }
}

function teamUpdatedLabel(item) {
  if (item.last_sync_at) {
    return ` · ${formatDateShort(item.last_sync_at)}`;
  }
  return '';
}

function renderSyncStatus(st) {
  const panel = $('syncProgress');
  const phaseEl = $('syncPhase');
  const detailEl = $('syncDetail');
  const metaEl = $('syncMeta');
  if (!panel) return;

  if (st.running || refreshPending) {
    panel.classList.remove('hidden');
    panel.classList.add('active');
    phaseEl.textContent = phaseLabels[st.phase] || st.phase || 'Обновление...';
    detailEl.textContent = st.detail || 'Загрузка...';
    metaEl.textContent = st.started_at ? `Начато: ${formatTime(st.started_at)} · можно переключать вкладки` : '';
    const isRefresh = cancellablePhases.has(st.phase);
    $('refreshBtn').disabled = st.running;
    if ($('refreshEventsBtn')) $('refreshEventsBtn').disabled = st.running;
    showStopRefresh(isRefresh && st.running);
    $('clearDbBtn').disabled = st.running;
    return;
  }

  showStopRefresh(false);
  $('clearDbBtn').disabled = false;
  panel.classList.remove('active');
  if (st.last_finished_at || st.last_result) {
    panel.classList.remove('hidden');
    phaseEl.textContent = st.phase === 'error' ? 'Ошибка' : 'Последнее обновление';
    if (st.phase === 'error') {
      detailEl.textContent = st.detail || 'Не удалось обновить';
    } else if (st.last_result) {
      const r = st.last_result;
      detailEl.textContent =
        `Турниров: ${r.events_synced}, команд: ${r.teams_saved}, матчей: ${r.matches_synced}, история: +${r.history_matches || 0}`;
    } else {
      detailEl.textContent = st.detail || 'Готово';
    }
    const parts = [];
    if (st.last_finished_at) parts.push(`Завершено: ${formatTime(st.last_finished_at)}`);
    if (st.last_refresh_at) parts.push(`В логе БД: ${formatTime(st.last_refresh_at)}`);
    metaEl.textContent = parts.join(' · ');
  } else {
    panel.classList.add('hidden');
  }
  $('refreshBtn').disabled = false;
  if ($('refreshEventsBtn')) $('refreshEventsBtn').disabled = false;
}

async function pollSyncStatus() {
  try {
    const st = await api('/api/sync/status');
    renderSyncStatus(st);

    const teams = st.teams ?? st.db?.teams ?? 0;
    const matches = st.matches ?? st.db?.matches ?? 0;
    const events = st.events ?? st.db?.events ?? 0;
    let dbLine = `В БД: ${teams} команд, ${matches} матчей, ${events} турниров`;
    if (st.last_refresh_at && !st.running) {
      dbLine += ` · обновлено ${formatTime(st.last_refresh_at)}`;
    }
    $('dbStats').textContent = dbLine;

    const snapshot = `${teams}|${matches}|${events}`;
    if (!st.running) {
      if (snapshot !== lastDbSnapshot) {
        lastDbSnapshot = snapshot;
        await reloadAllLists();
      } else if (wasSyncRunning) {
        await reloadAllLists();
      }
    }
    wasSyncRunning = st.running;

    refreshPending = false;
    if (st.running) {
      ensureSyncPolling();
    }
  } catch (err) {
    refreshPending = false;
    $('dbStats').textContent = err.message || 'Сервер не запущен';
    $('refreshStatus').textContent = err.message || 'Сервер не запущен';
    $('refreshBtn').disabled = false;
    showStopRefresh(false);
    if (syncPollTimer) {
      clearInterval(syncPollTimer);
      syncPollTimer = null;
    }
  }
}

async function loadDbStats() {
  await pollSyncStatus();
}

async function loadEvents(search = '') {
  const q = search ? `?search=${encodeURIComponent(search)}&limit=80` : '?limit=80';
  const events = asArray(await api(`/api/events${q}`));
  fillEvents(events);
  return events;
}

function filterTeamsBySearch(teams, query) {
  const q = (query || '').trim().toLowerCase();
  if (!q) return teams;
  return teams.filter((t) => t.name.toLowerCase().includes(q));
}

function applyTeamFilter() {
  const q = $('teamSearch').value.trim();
  const filtered = filterTeamsBySearch(eventTeamsCache, q);
  fillSelect($('team1Select'), filtered, '— нет команд в турнире —');
  fillSelect($('team2Select'), filtered, '— нет команд в турнире —');
}

async function loadEventTeams(eventId) {
  if (!eventId) {
    await loadAllTeamsForPredict();
    return;
  }
  $('status').textContent = 'Загрузка команд турнира...';
  const teams = asArray(await api(`/api/events/${eventId}/teams`));
  eventTeamsCache = teams;
  $('teamSearch').disabled = false;
  applyTeamFilter();
  $('status').textContent = teams.length
    ? `Участников турнира: ${teams.length}`
    : 'Нет команд — нажмите «Обновить турниры и команды»';
}

async function loadProfileTeamList(search = '') {
  const select = $('profileTeamSelect');
  if (!select) return;
  const q = search ? `?search=${encodeURIComponent(search)}&limit=100` : '?limit=100';
  $('profileStatus').textContent = 'Загрузка списка команд...';
  const teams = asArray(await api(`/api/teams${q}`));
  fillSelect(select, teams, '— нет команд в БД —');
  $('profileStatus').textContent = teams.length
    ? `${teams.length} команд в БД — выберите и нажмите «Показать из БД»`
    : 'Нет команд в БД — нажмите «Обновить турниры и команды»';
}

function renderProfileMeta(p) {
  const parts = [];
  if (p.last_sync_at) {
    parts.push(`Обновлено: ${formatTime(p.last_sync_at)}`);
  } else {
    parts.push('Ещё не загружалось с HLTV');
  }
  if (p.players_count != null) parts.push(`игроков: ${p.players_count}`);
  if (p.matches_total != null) parts.push(`матчей: ${p.matches_total}`);
  if (p.map_stats_count != null) parts.push(`карт: ${p.map_stats_count}`);
  $('profileMeta').textContent = parts.join(' · ');
  $('refreshProfileBtn').disabled = false;
  if (p.stale) {
    $('refreshProfileBtn').classList.add('primary');
    $('refreshProfileBtn').classList.remove('muted-btn');
  } else {
    $('refreshProfileBtn').classList.remove('primary');
  }
}

function renderTeamMapStatsBlock(labelId, gridId, teamName, stats) {
  const label = $(labelId);
  const grid = $(gridId);
  if (!label || !grid) return;
  label.textContent = teamName || 'Команда';
  grid.innerHTML = '';
  if (!stats?.length) {
    grid.textContent = 'Нет данных по картам — обновите команду с HLTV';
    return;
  }
  for (const m of stats) {
    const div = document.createElement('div');
    div.className = 'map-card';
    const total = (m.wins || 0) + (m.losses || 0);
    let line = `${m.wins}В / ${m.losses}П · ${Number(m.win_rate || 0).toFixed(0)}% (${total})`;
    if (m.pick_rate > 0) line += ` · pick ${(m.pick_rate * 100).toFixed(0)}%`;
    if (m.ban_rate > 0) line += ` · ban ${(m.ban_rate * 100).toFixed(0)}%`;
    div.innerHTML = `<strong>${m.map_name}</strong><br>${line}`;
    grid.appendChild(div);
  }
}

function renderTeamProfile(p) {
  $('teamProfile').classList.remove('hidden');
  const rank = p.team.world_rank ? ` · мир #${p.team.world_rank}` : '';
  const rating = p.team.hltv_rating ? ` · рейтинг ${Math.round(p.team.hltv_rating)}` : '';
  $('profileTitle').textContent = `${p.team.name}${rank}${rating}`;
  renderProfileMeta(p);

  const summary = $('profileSummary');
  summary.innerHTML = '';
  const cards = [
    ['Матчей', p.matches_total],
    ['Побед', p.wins],
    ['Поражений', p.losses],
    ['Винрейт', `${p.win_rate.toFixed(1)}%`],
    ['Форма (10)', `${p.recent_wins}/${p.recent_total}`],
    ['Винрейт (10)', `${p.recent_win_rate.toFixed(1)}%`],
  ];
  for (const [label, val] of cards) {
    const div = document.createElement('div');
    div.className = 'stat-card';
    div.innerHTML = `<strong>${val}</strong><span>${label}</span>`;
    summary.appendChild(div);
  }

  const players = $('profilePlayers');
  players.innerHTML = '';
  if (!p.players?.length) {
    const li = document.createElement('li');
    li.textContent = 'Нет данных об игроках';
    players.appendChild(li);
  } else {
    for (const pl of p.players) {
      const li = document.createElement('li');
      li.textContent = pl.name;
      players.appendChild(li);
    }
  }

  const maps = $('profileMaps');
  maps.innerHTML = '';
  if (!p.map_stats?.length) {
    maps.textContent = 'Нет статистики по картам';
  } else {
    for (const m of p.map_stats) {
      const div = document.createElement('div');
      div.className = 'map-card';
      const total = m.wins + m.losses;
      let line = `${m.wins}В / ${m.losses}П · ${Number(m.win_rate || 0).toFixed(0)}% (${total})`;
      if (m.pick_rate > 0) line += ` · pick ${(m.pick_rate * 100).toFixed(0)}%`;
      if (m.ban_rate > 0) line += ` · ban ${(m.ban_rate * 100).toFixed(0)}%`;
      div.innerHTML = `<strong>${m.map_name}</strong><br>${line}`;
      maps.appendChild(div);
    }
  }

  const events = $('profileEvents');
  events.innerHTML = '';
  if (!p.events?.length) {
    const li = document.createElement('li');
    li.textContent = 'Нет данных по турнирам';
    events.appendChild(li);
  } else {
    for (const e of p.events) {
      const li = document.createElement('li');
      const name = e.event_name || `Турнир ${e.event_id}`;
      li.textContent = `${name}: ${e.wins}В / ${e.matches - e.wins}П (${e.matches} матч.)`;
      events.appendChild(li);
    }
  }

  const matches = $('profileMatches');
  matches.innerHTML = '';
  if (!p.recent_matches?.length) {
    matches.textContent = 'Нет матчей в БД';
  } else {
    for (const m of p.recent_matches) {
      const div = document.createElement('div');
      const played = m.played !== false;
      div.className = 'match-row ' + (played ? (m.won ? 'win' : 'loss') : 'scheduled');
      const result = played ? (m.won ? 'Победа' : 'Поражение') : 'Предстоящий матч';
      const meta = [m.date, m.format?.toUpperCase(), m.event_name].filter(Boolean).join(' · ');
      div.innerHTML = `<span>${m.date || '—'}</span><span><strong>${result}</strong> vs ${m.opponent}<br><span class="muted">${meta}</span></span><span>#${m.id}</span>`;
      matches.appendChild(div);
    }
  }
}

function normalizeCookiePair(chunk) {
  chunk = chunk.trim().replace(/^cookie:\s*/i, '');
  if (!chunk) return '';
  for (const sep of [':', '=']) {
    const idx = chunk.indexOf(sep);
    if (idx > 0) {
      const name = chunk.slice(0, idx).trim();
      const value = chunk.slice(idx + 1).trim();
      if (name && value && (name === 'cf_clearance' || name === '__cf_bm' || name.startsWith('cf_') || name.startsWith('__cf'))) {
        return `${name}=${value}`;
      }
    }
  }
  return '';
}

function stripCookieValue(name, raw) {
  const text = (raw || '').trim();
  if (!text) return '';
  const lower = text.toLowerCase();
  const n = name.toLowerCase();
  for (const sep of ['=', ':']) {
    const prefix = n + sep;
    if (lower.startsWith(prefix)) {
      return text.slice(name.length + 1).trim();
    }
  }
  return text;
}

function parseCookieParts(cookie) {
  let cf = '';
  let bm = '';
  for (const chunk of cookie.split(/[;\n]/)) {
    const part = chunk.trim();
    if (!part) continue;
    const eq = part.indexOf('=');
    if (eq <= 0) continue;
    const name = part.slice(0, eq).trim();
    const value = part.slice(eq + 1).trim();
    if (name === 'cf_clearance') cf = value;
    if (name === '__cf_bm') bm = value;
  }
  return { cf, bm };
}

function extractCookie(text) {
  const raw = (text || '').trim();
  if (!raw) return '';
  const curlPatterns = [
    /(?:-H|--header)\s+['"]cookie:\s*([^'"]+)['"]/i,
    /(?:-H|--header)\s+['"]Cookie:\s*([^'"]+)['"]/i,
  ];
  for (const re of curlPatterns) {
    const m = raw.match(re);
    if (m) return normalizeCookieText(m[1].trim());
  }
  const lineMatch = raw.match(/^\s*cookie:\s*(.+)$/im);
  if (lineMatch) return normalizeCookieText(lineMatch[1].trim());
  return normalizeCookieText(raw);
}

function buildCookieFromFields() {
  const cf = stripCookieValue('cf_clearance', $('cfClearanceInput').value);
  const bm = stripCookieValue('__cf_bm', $('cfBmInput').value);
  if (!cf) return '';
  if (!bm) return `cf_clearance=${cf}`;
  return `cf_clearance=${cf}; __cf_bm=${bm}`;
}

function fillCookieFields(cf, bm) {
  if (cf) $('cfClearanceInput').value = cf;
  if (bm) $('cfBmInput').value = bm;
}

function applyCookiePaste() {
  const raw = ($('cookiePasteInput').value || '').trim();
  if (!raw) return;
  const combined = extractCookie(raw);
  const { cf, bm } = parseCookieParts(combined);
  if (cf || bm) {
    fillCookieFields(cf, bm);
    $('cookiePasteInput').value = '';
  }
}

function normalizeCookieText(raw) {
  const chunks = raw.split(/\n|;/).map((s) => s.trim()).filter(Boolean);
  const parts = chunks.map(normalizeCookiePair).filter(Boolean);
  if (parts.length) return parts.join('; ');
  const one = raw.replace(/\n/g, '').trim();
  if (!one.includes('=') && !one.includes(':') && one.length > 40) {
    return `cf_clearance=${one}`;
  }
  return one.replace(/^cookie:\s*/i, '').trim();
}

async function loadCookieStatus() {
  try {
    const st = await api('/api/cookie/status');
    const el = $('cookieStatus');
    if (st.valid) {
      el.textContent = '✓ Cookie сохранён и работает';
      el.style.color = '#4ade80';
    } else if (st.has_cookie) {
      el.textContent = 'Cookie устарел — скопируйте новый';
      el.style.color = '#fbbf24';
    } else {
      el.textContent = 'Cookie не задан';
      el.style.color = '';
    }
  } catch (_) {}
}

function renderPrediction(p) {
  $('results').classList.remove('hidden');
  const fmt = (p.format || '').toUpperCase();
  $('matchTitle').textContent = `${p.team1.name} vs ${p.team2.name} (${fmt})`;
  const conf = confidenceRu[p.confidence] || p.confidence;
  $('confidence').textContent = `Уверенность: ${conf}`;

  $('team1Label').textContent = p.team1.name;
  $('team2Label').textContent = p.team2.name;
  $('team1Pct').textContent = `${p.win_prob.team1}%`;
  $('team2Pct').textContent = `${p.win_prob.team2}%`;
  $('team1Bar').style.width = `${p.win_prob.team1}%`;
  $('team2Bar').style.width = `${p.win_prob.team2}%`;

  const bd = $('breakdown');
  bd.innerHTML = '';
  const b = p.breakdown;
  for (const [key, label] of Object.entries(breakdownRu)) {
    const li = document.createElement('li');
    li.textContent = `${label}: ${b[key]}%`;
    bd.appendChild(li);
  }

  const series = $('series');
  series.innerHTML = '';
  for (const [score, pct] of Object.entries(p.series_scores || {})) {
    const li = document.createElement('li');
    li.textContent = `${score}: ${pct}%`;
    series.appendChild(li);
  }

  const veto = $('veto');
  veto.innerHTML = '';
  for (const s of (p.veto?.steps || [])) {
    const li = document.createElement('li');
    const action = vetoActionRu[s.action] || s.action;
    li.textContent = `${s.order}. ${action}: ${s.team_name || '—'} → ${s.map_name}`;
    veto.appendChild(li);
  }

  renderTeamMapStatsBlock('team1MapLabel', 'team1MapStats', p.team1?.name, p.team1_map_stats);
  renderTeamMapStatsBlock('team2MapLabel', 'team2MapStats', p.team2?.name, p.team2_map_stats);

  const maps = $('maps');
  maps.innerHTML = '';
  for (const m of p.maps || []) {
    const div = document.createElement('div');
    div.className = 'map-card' + (m.in_series ? ' series' : '');
    const role = m.role ? ` · ${vetoActionRu[m.role] || m.role}` : '';
    div.innerHTML = `<strong>${m.map_name}</strong><br>${m.team1_win_pct}%${role}`;
    maps.appendChild(div);
  }

  const h2hSummary = $('h2hSummary');
  const h2hMatches = $('h2hMatches');
  h2hSummary.textContent = '';
  h2hMatches.innerHTML = '';
  const h2h = p.h2h || {};
  if (h2h.total > 0) {
    h2hSummary.textContent =
      `${h2h.team1_wins} : ${h2h.team2_wins} (${h2h.total} матч.) · винрейт ${p.team1.name}: ${h2h.team1_win_pct}%`;
    for (const m of h2h.matches || []) {
      const div = document.createElement('div');
      const played = m.played !== false && m.winner_name;
      div.className = 'match-row ' + (played ? (m.team1_won ? 'win' : 'loss') : 'scheduled');
      const result = played
        ? (m.winner_name ? `Победа ${m.winner_name}` : (m.team1_won ? `Победа ${p.team1.name}` : `Победа ${p.team2.name}`))
        : 'Предстоящий матч';
      const meta = [m.date, m.format?.toUpperCase(), m.event].filter(Boolean).join(' · ');
      div.innerHTML =
        `<span>${m.date || '—'}</span><span><strong>${result}</strong><br>${m.team1_name} vs ${m.team2_name}<br><span class="muted">${meta}</span></span><span>#${m.id}</span>`;
      h2hMatches.appendChild(div);
    }
  } else {
    h2hSummary.textContent = 'Личных встреч в БД нет — обновите данные команд';
  }
}

const logPanel = $('logPanel');
const maxLogLines = 300;

function appendLog(line) {
  if (!logPanel) return;
  logPanel.textContent += line + '\n';
  const lines = logPanel.textContent.split('\n');
  if (lines.length > maxLogLines) {
    logPanel.textContent = lines.slice(-maxLogLines).join('\n');
  }
  logPanel.scrollTop = logPanel.scrollHeight;
}

function startLogStream() {
  const es = new EventSource('/api/logs/stream');
  es.onmessage = (e) => appendLog(e.data);
  es.onerror = () => {
    es.close();
    setTimeout(startLogStream, 3000);
  };
}

$('openHltvBtn').addEventListener('click', async () => {
  try {
    await api('/api/hltv/open', { method: 'POST' });
    $('cookieStatus').textContent = 'Chrome PSR открыт — скопируйте cookie и сохраните';
    $('cookieStatus').style.color = '#fbbf24';
  } catch (err) {
    $('cookieStatus').textContent = err.message;
  }
});

$('connectBtn').addEventListener('click', async () => {
  $('connectBtn').disabled = true;
  $('cookieStatus').textContent = 'Проверка cookie...';
  try {
    await api('/api/connect', { method: 'POST' });
    $('cookieStatus').textContent = '✓ Cookie работает';
    $('cookieStatus').style.color = '#4ade80';
  } catch (err) {
    $('cookieStatus').textContent = err.message;
    $('cookieStatus').style.color = '#f87171';
  } finally {
    $('connectBtn').disabled = false;
  }
});

['cfClearanceInput', 'cfBmInput'].forEach((id) => {
  $(id).addEventListener('blur', () => {
    const el = $(id);
    if (id === 'cfClearanceInput') {
      el.value = stripCookieValue('cf_clearance', el.value);
    } else {
      el.value = stripCookieValue('__cf_bm', el.value);
    }
  });
});

$('cookiePasteInput').addEventListener('paste', () => setTimeout(applyCookiePaste, 0));
$('cookiePasteInput').addEventListener('blur', applyCookiePaste);

$('saveCookieBtn').addEventListener('click', async () => {
  applyCookiePaste();
  const cookie = buildCookieFromFields();
  if (!cookie || !cookie.includes('cf_clearance=')) {
    $('cookieStatus').textContent = 'Заполните cf_clearance и __cf_bm';
    $('cookieStatus').style.color = '#fbbf24';
    return;
  }
  $('saveCookieBtn').disabled = true;
  try {
    await api('/api/cookie', { method: 'POST', body: JSON.stringify({ cookie }) });
    $('cookieStatus').textContent = '✓ Cookie сохранён';
    $('cookieStatus').style.color = '#4ade80';
    $('refreshStatus').textContent = 'Можно обновить турниры и команды';
  } catch (err) {
    $('cookieStatus').textContent = err.message;
    $('cookieStatus').style.color = '#f87171';
  } finally {
    $('saveCookieBtn').disabled = false;
  }
});

bindClick('refreshEventsBtn', async () => {
  $('refreshStatus').textContent = 'Обновление списка турниров...';
  $('refreshEventsBtn').disabled = true;
  $('refreshBtn').disabled = true;
  showSyncProgress(true);
  try {
    const res = await api('/api/refresh/events', { method: 'POST' });
    $('refreshStatus').textContent = res.started === false
      ? 'Другая операция уже выполняется'
      : 'Загрузка турниров с HLTV...';
    await pollSyncStatus();
    if (!syncPollTimer) syncPollTimer = setInterval(pollSyncStatus, 1500);
  } catch (err) {
    $('refreshStatus').textContent = err.message || 'Ошибка';
    $('refreshEventsBtn').disabled = false;
    $('refreshBtn').disabled = false;
  }
});

bindClick('refreshBtn', async () => {
  refreshPending = true;
  $('refreshStatus').textContent = 'Запуск обновления...';
  $('refreshBtn').disabled = true;
  showStopRefresh(true);
  showSyncProgress(true);
  try {
    const res = await api('/api/refresh', { method: 'POST' });
    $('refreshStatus').textContent = res.started === false
      ? 'Обновление уже выполняется'
      : 'Обновление запущено — смотрите статус ниже';
    await pollSyncStatus();
    if (!syncPollTimer) syncPollTimer = setInterval(pollSyncStatus, 1500);
  } catch (err) {
    refreshPending = false;
    $('refreshStatus').textContent = err.message || 'Ошибка';
    $('refreshBtn').disabled = false;
    showStopRefresh(false);
  }
});

bindClick('stopRefreshBtn', async () => {
  $('stopRefreshBtn').disabled = true;
  $('refreshStatus').textContent = 'Остановка обновления...';
  try {
    await api('/api/sync/stop', { method: 'POST' });
    $('refreshStatus').textContent = 'Остановка запрошена — дождитесь завершения текущего шага';
    await pollSyncStatus();
    if (!syncPollTimer) syncPollTimer = setInterval(pollSyncStatus, 1500);
  } catch (err) {
    $('refreshStatus').textContent = err.message || 'Не удалось остановить';
    $('stopRefreshBtn').disabled = false;
  }
});

bindClick('clearDbBtn', async () => {
  if (!confirm('Удалить все данные из БД (команды, матчи, турниры)? Cookie сохранится.')) return;
  if ($('clearDbBtn').disabled) return;
  $('clearDbBtn').disabled = true;
  $('refreshBtn').disabled = true;
  $('refreshStatus').textContent = 'Очистка БД...';
  try {
    const res = await api('/api/db/clear', { method: 'POST' });
    const db = res.db || {};
    $('refreshStatus').textContent =
      `БД очищена: ${db.teams || 0} команд, ${db.matches || 0} матчей, ${db.events || 0} турниров. Нажмите «Обновить».`;
    eventTeamsCache = [];
    $('eventSelect').value = '';
    $('eventSearch').value = '';
    lastDbSnapshot = '';
    await loadDbStats();
    await reloadAllLists();
    $('teamProfile').classList.add('hidden');
    $('results').classList.add('hidden');
  } catch (err) {
    $('refreshStatus').textContent = `Ошибка очистки: ${err.message}`;
  } finally {
    $('clearDbBtn').disabled = false;
    $('refreshBtn').disabled = false;
  }
});

$('eventSearch').addEventListener('input', () => {
  clearTimeout(eventSearchTimer);
  eventSearchTimer = setTimeout(async () => {
    try {
      await loadEvents($('eventSearch').value.trim());
    } catch (err) {
      $('status').textContent = err.message;
    }
  }, 350);
});

$('eventSelect').addEventListener('change', async (e) => {
  $('teamSearch').value = '';
  try {
    await loadEventTeams(e.target.value);
  } catch (err) {
    $('status').textContent = err.message;
  }
});

$('teamSearch').addEventListener('input', () => {
  clearTimeout(teamSearchTimer);
  teamSearchTimer = setTimeout(applyTeamFilter, 200);
});

$('profileTeamSearch').addEventListener('input', () => {
  clearTimeout(profileSearchTimer);
  profileSearchTimer = setTimeout(async () => {
    try {
      await loadProfileTeamList($('profileTeamSearch').value.trim());
    } catch (err) {
      $('profileStatus').textContent = err.message;
    }
  }, 350);
});

async function loadTeamProfile(teamId, sync = false) {
  const q = sync ? '?sync=1' : '';
  const profile = await api(`/api/teams/${teamId}/profile${q}`);
  renderTeamProfile(profile);
  if (profile.sync_error) {
    $('profileStatus').textContent = `Частичные данные: ${profile.sync_error}`;
  } else if (sync && profile.matches_total === 0) {
    $('profileStatus').textContent = 'Матчей в БД нет — проверьте cookie и повторите';
  } else if (sync) {
    $('profileStatus').textContent =
      `Обновлено: ${profile.matches_total} матчей, ${profile.players?.length || 0} игроков, ${profile.map_stats?.length || 0} карт`;
  } else if (profile.stale) {
    $('profileStatus').textContent = 'Данные устарели — нажмите «Обновить с HLTV»';
  } else {
    $('profileStatus').textContent = 'Данные из БД';
  }
  lastDbSnapshot = '';
  await loadDbStats();
  await loadProfileTeamList($('profileTeamSearch').value.trim());
}

bindClick('showProfileBtn', async () => {
  const teamId = parseInt($('profileTeamSelect').value, 10);
  if (!teamId) {
    $('profileStatus').textContent = 'Выберите команду';
    return;
  }
  $('showProfileBtn').disabled = true;
  try {
    await loadTeamProfile(teamId, false);
  } catch (err) {
    $('profileStatus').textContent = err.message;
  } finally {
    $('showProfileBtn').disabled = false;
  }
});

bindClick('refreshProfileBtn', async () => {
  const teamId = parseInt($('profileTeamSelect').value, 10);
  if (!teamId) {
    $('profileStatus').textContent = 'Выберите команду';
    return;
  }
  $('profileStatus').textContent = 'Загрузка с HLTV — можно переключать вкладки, прогресс сверху';
  $('refreshProfileBtn').disabled = true;
  ensureSyncPolling();
  try {
    const res = await api(`/api/teams/${teamId}/sync`, { method: 'POST' });
    if (res.started === false && !res.status?.running) {
      $('profileStatus').textContent = 'Другая операция уже выполняется';
      return;
    }
    await waitForOperation();
    await loadTeamProfile(teamId, false);
    $('profileStatus').textContent = 'Обновлено с HLTV';
  } catch (err) {
    $('profileStatus').textContent = err.message;
  } finally {
    $('refreshProfileBtn').disabled = false;
  }
});

bindClick('syncAllTeamsBtn', async () => {
  $('profileStatus').textContent = 'Запуск обновления всех команд...';
  $('syncAllTeamsBtn').disabled = true;
  try {
    const res = await api('/api/teams/sync-all', { method: 'POST' });
    $('profileStatus').textContent = res.started === false
      ? 'Обновление уже выполняется'
      : 'Обновление всех команд запущено — смотрите статус на вкладке «Данные»';
    await pollSyncStatus();
    if (!syncPollTimer) syncPollTimer = setInterval(pollSyncStatus, 1500);
  } catch (err) {
    $('profileStatus').textContent = err.message;
  } finally {
    $('syncAllTeamsBtn').disabled = false;
  }
});

bindClick('predictBtn', async () => {
  const team1 = parseInt($('team1Select').value, 10);
  const team2 = parseInt($('team2Select').value, 10);
  const format = $('formatSelect').value;

  if (!team1 || !team2) {
    $('status').textContent = 'Выберите обе команды из списка турнира';
    return;
  }
  if (team1 === team2) {
    $('status').textContent = 'Команды должны быть разными';
    return;
  }

  $('status').textContent = 'Анализ... (можно переключать вкладки — прогресс сверху)';
  $('predictBtn').disabled = true;
  ensureSyncPolling();

  try {
    const pred = await api('/api/predict', {
      method: 'POST',
      body: JSON.stringify({ team1_id: team1, team2_id: team2, format }),
    });
    renderPrediction(pred);
    $('status').textContent = '';
    lastDbSnapshot = '';
    await loadDbStats();
  } catch (err) {
    $('status').textContent = err.message;
  } finally {
    $('predictBtn').disabled = false;
  }
});

async function bootstrap() {
  ensureSyncPolling();
  await pollSyncStatus().catch(() => {});
  await reloadAllLists().catch((err) => {
    const el = $('profileStatus');
    if (el) el.textContent = err.message || 'Не удалось загрузить списки';
  });
}

function initApp() {
  if (location.protocol === 'file:') {
    $('refreshStatus').textContent =
      'Откройте http://127.0.0.1:8080 в браузере (после запуска run.bat), не файл index.html';
  }
  initTabs();
  loadCookieStatus().catch(() => {});
  bootstrap().catch((err) => {
    const el = $('profileStatus');
    if (el) el.textContent = err.message || 'Ошибка инициализации';
  });
  fetch('/api/logs').then((r) => r.json()).then((lines) => lines.forEach(appendLog)).catch(() => {});
  startLogStream();
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', initApp);
} else {
  initApp();
}
