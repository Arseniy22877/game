const BASE_COST = 5;
let state = null;
let values = new Map();
let selectedStrategies = new Set();
let selectedHints = new Set();

const $ = (id) => document.getElementById(id);
const money = (value) => `${value} кредитов`;
const esc = (value) => String(value).replace(/[&<>"']/g, (c) => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));

async function request(path, options = {}) {
  const response = await fetch(path, { headers: {'Content-Type': 'application/json'}, ...options });
  if (!response.ok) throw new Error(await response.text());
  const data = await response.json();
  return {
    ...data,
    basicAttributes: Array.isArray(data.basicAttributes) ? data.basicAttributes : [],
    attributes: Array.isArray(data.attributes) ? data.attributes : [],
    strategies: Array.isArray(data.strategies) ? data.strategies : [],
    hints: Array.isArray(data.hints) ? data.hints : [],
    newEvents: Array.isArray(data.newEvents) ? data.newEvents : [],
    history: Array.isArray(data.history) ? data.history : [],
  };
}

function render() {
  $('money').textContent = `${projectedMoney()} / ${state.maxMoney}`;
  $('qp').textContent = `${state.qp} очков`;
  $('cycle').textContent = `ЦИКЛ ${state.cycle}`;
  renderBasicAttributes(); renderAttributes(); renderChoices(); renderEvents(); updateCost();
}

function projectedMoney() {
  let cost = BASE_COST;
  state.attributes.forEach((attribute) => { cost += Math.round((values.get(attribute.id) ?? attribute.value) * attribute.costPerUnit); });
  state.strategies.forEach((item) => { if (selectedStrategies.has(item.id)) cost += item.cost; });
  state.hints.forEach((item) => { if (selectedHints.has(item.id)) cost += item.cost; });
  return state.money - cost;
}

function renderAttributes() {
  $('attributes').innerHTML = state.attributes.map((attribute) => {
    const value = values.get(attribute.id) ?? attribute.value;
    return `<label class="attribute-row"><span class="attribute-name"><strong>${esc(attribute.name)}</strong><small>${esc(attribute.description)}</small></span><input data-id="${attribute.id}" type="range" min="${attribute.min}" max="${attribute.max}" step=".01" value="${value}"><span class="value">${Number(value).toFixed(2)}</span></label>`;
  }).join('');
  document.querySelectorAll('#attributes input').forEach((input) => input.addEventListener('input', (event) => {
    values.set(Number(event.target.dataset.id), Number(event.target.value)); render();
  }));
}

function renderBasicAttributes() {
  $('basic-attributes').innerHTML = state.basicAttributes.map((attribute) => `<div class="basic-attribute"><div><strong>${esc(attribute.name)}</strong><small>${esc(attribute.description)}</small></div><span>${Number(attribute.value).toFixed(2)}</span></div>`).join('');
}

function renderChoices() {
  $('strategies').innerHTML = state.strategies.map((item) => choiceMarkup(item, 'strategy')).join('');
  $('hints').innerHTML = state.hints.map((item) => choiceMarkup(item, 'hint')).join('');
  document.querySelectorAll('.choice input').forEach((input) => input.addEventListener('change', (event) => {
    const set = event.target.dataset.kind === 'strategy' ? selectedStrategies : selectedHints;
    event.target.checked ? set.add(Number(event.target.value)) : set.delete(Number(event.target.value)); render();
  }));
}

function choiceMarkup(item, kind) {
  const unlocked = item.unlocked && !(kind === 'hint' && item.used);
  const set = kind === 'strategy' ? selectedStrategies : selectedHints;
  const checked = set.has(item.id);
  const extra = kind === 'hint' ? `<span class="choice-cost">${item.reliability} надёжность</span>${item.secret ? `<span class="hint-secret">${esc(item.secret)}</span>` : ''}` : '';
  return `<label class="choice ${checked ? 'selected' : ''} ${unlocked ? '' : 'locked'}"><input type="${kind === 'strategy' ? 'checkbox' : 'checkbox'}" data-kind="${kind}" value="${item.id}" ${checked ? 'checked' : ''} ${unlocked ? '' : 'disabled'}><span class="choice-title">${esc(item.name)}<span class="choice-cost">−${item.cost}</span></span><span class="choice-description">${esc(item.description)}</span>${extra}</label>`;
}

function renderEvents() {
  $('new-events').innerHTML = state.newEvents.length ? state.newEvents.map((event) => eventMarkup(event, 'произошло')).join('') : '<p class="empty-state">В этом цикле событий не произошло.</p>';
  $('history').innerHTML = state.history.length ? state.history.map((event) => eventMarkup(event, 'завершено')).join('') : '<p class="empty-state">История пока пуста.</p>';
}

function eventMarkup(event, status) {
  return `<article class="event ${event.positive ? 'good' : 'bad'}"><div class="event-top"><span>${event.positive ? '✦' : '△'} ${esc(event.name)}</span><span class="event-status">${status}</span></div><p class="event-description">${esc(event.description)}</p></article>`;
}

function updateCost() {
  let attributes = 0;
  state.attributes.forEach((item) => { attributes += Math.round((values.get(item.id) ?? item.value) * item.costPerUnit); });
  const strategies = [...selectedStrategies].reduce((sum, id) => sum + state.strategies.find((item) => item.id === id).cost, 0);
  const hints = [...selectedHints].reduce((sum, id) => sum + state.hints.find((item) => item.id === id).cost, 0);
  $('attribute-cost').textContent = money(attributes); $('strategy-cost').textContent = money(strategies); $('hint-cost').textContent = money(hints);
  const total = BASE_COST + attributes + strategies + hints;
  $('total-cost').textContent = money(total);
  $('commit').disabled = total > state.money;
}

async function commit() {
  const attributes = state.attributes.map((item) => ({id: item.id, value: values.get(item.id) ?? item.value}));
  let cost = BASE_COST;
  state.attributes.forEach((item) => { cost += Math.round((values.get(item.id) ?? item.value) * item.costPerUnit); });
  state.strategies.forEach((item) => { if (selectedStrategies.has(item.id)) cost += item.cost; });
  state.hints.forEach((item) => { if (selectedHints.has(item.id)) cost += item.cost; });
  try {
    state = await request('/api/cycle', {method: 'POST', body: JSON.stringify({attributes, strategies: [...selectedStrategies], hints: [...selectedHints], cost})});
    values = new Map(); selectedHints.clear(); $('notice').textContent = ''; render();
  } catch (error) { $('notice').textContent = error.message; }
}

async function reset() { state = await request('/api/reset', {method: 'POST'}); values = new Map(); selectedStrategies.clear(); selectedHints.clear(); $('notice').textContent = ''; render(); }

async function init() {
  try {
    state = await request('/api/state');
    render();
  } catch (error) {
    $('notice').textContent = `Не удалось загрузить игру: ${error.message}`;
  }
}
$('commit').addEventListener('click', commit); $('reset').addEventListener('click', reset); init();
