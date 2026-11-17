// Локальный мок-сервер бэкенда для тестирования фронтенда (порт 8080)
// Полная изоляция команд, динамический пересчет показателей, бюджета и генерация уникальных событий.
// Запуск: node mock-server.js

import http from 'http';
import url from 'url';
import { WebSocketServer, WebSocket } from 'ws';

let gameState = {
  gameId: 1,
  phase: 'lobby', // 'none' | 'lobby' | 'round_open' | 'round_closed' | 'finished'
  round: 0,
  scenarioId: 'demo',
  baseRoundCost: 5,
};

let teams = [
  {
    id: 1,
    name: 'Альфа',
    members: ['Алексей Иванов', 'Мария Смирнова'],
    ready: false,
    online: false,
    submitted: false,
    bankrupt: false,
    token: 'token-alpha-123',
  },
  {
    id: 2,
    name: 'Бета',
    members: ['Дмитрий Кузнецов', 'Елена Попова'],
    ready: true,
    online: false,
    submitted: false,
    bankrupt: false,
    token: 'token-beta-456',
  },
];

// Каталог возможных событий симуляции
const EVENT_CATALOG = {
  101: { id: 101, name: 'Инвестиционный грант', description: 'Венчурный фонд выделил грант на масштабирование (+25K $, +10 Доверие).', imageUrl: '', isPositive: true, money: 25, qp: 0, effects: { 1: 10 } },
  102: { id: 102, name: 'Хакерская атака на сервера', description: 'Уязвимость в API привела к кратковременному простою (-15 Качество, -10 Доверие).', imageUrl: '', isPositive: false, money: 0, qp: 0, effects: { 1: -10, 2: -15 } },
  103: { id: 103, name: 'Вирусный пост в соцсетях', description: 'Пользовательский обзор разлетелся по соцсетям (+15 Доля рынка, +8 Доверие).', imageUrl: '', isPositive: true, money: 0, qp: 0, effects: { 1: 8, 4: 15 } },
  104: { id: 104, name: 'Сбой платежного шлюза', description: 'Партнёр по эквайрингу задержал переводы (-10K $, -5 Доверие).', imageUrl: '', isPositive: false, money: -10, qp: 0, effects: { 1: -5 } },
  105: { id: 105, name: 'Внутренний хакатон команды', description: 'Разработчики за выходные собрали ключевую фичу (+15 Качество, +5 QP).', imageUrl: '', isPositive: true, money: 0, qp: 5, effects: { 2: 15 } },
  106: { id: 106, name: 'Уход ведущего разработчика', description: 'Тимлид перешел в зарубежный стартап (-15 Мотивация, -5 Качество).', imageUrl: '', isPositive: false, money: 0, qp: 0, effects: { 2: -5, 3: -15 } },
  107: { id: 107, name: 'Стратегический B2B-контракт', description: 'Подписан годовой контракт с крупным ритейлером (+35K $, +12 Доля рынка).', imageUrl: '', isPositive: true, money: 35, qp: 0, effects: { 4: 12 } },
  108: { id: 108, name: 'Претензия антимонопольной службы', description: 'Юридические разбирательства и аудит (-20K $, -8 Доверие).', imageUrl: '', isPositive: false, money: -20, qp: 0, effects: { 1: -8 } },
  109: { id: 109, name: 'Позитивный обзор отраслевого лидера', description: 'Эксперт года назвал ваш продукт самым многообещающим (+15 Доверие, +10 Доля рынка).', imageUrl: '', isPositive: true, money: 0, qp: 0, effects: { 1: 15, 4: 10 } },
  110: { id: 110, name: 'Рост цен на облачные сервера', description: 'Датацентр поднял тарифы на 20% (-15K $).', imageUrl: '', isPositive: false, money: -15, qp: 0, effects: {} },
};

function createInitialTeamState(teamId) {
  return {
    budget: { money: 100, maxMoney: 150, qp: 10 },
    attributes: [
      { id: 1, name: 'Показатель 1 (Доверие)', description: 'Доверие клиентов и партнеров', value: 50 + (teamId * 3), min: 0, max: 100 },
      { id: 2, name: 'Показатель 2 (Продукт)', description: 'Качество и стабильность продукта', value: 45 + (teamId * 2), min: 0, max: 100 },
      { id: 3, name: 'Показатель 3 (Команда)', description: 'Мотивация и продуктивность команды', value: 60 - (teamId * 2), min: 0, max: 100 },
      { id: 4, name: 'Показатель 4 (Доля рынка)', description: 'Охват рынка и узнаваемость', value: 30 + (teamId * 4), min: 0, max: 100 },
    ],
    configurable: [
      { id: 10, name: 'Сфера 1: Маркетинг', description: 'Привлечение клиентов и реклама', value: 0.3, min: 0, max: 1, costPerUnit: 100 },
      { id: 11, name: 'Сфера 2: Разработка', description: 'Развитие ключевых функций платформы', value: 0.3, min: 0, max: 1, costPerUnit: 100 },
      { id: 12, name: 'Сфера 3: Продажи', description: 'Каналы продаж и клиентский сервис', value: 0.2, min: 0, max: 1, costPerUnit: 100 },
      { id: 13, name: 'Сфера 4: Исследования', description: 'R&D и клиентские интервью', value: 0.2, min: 0, max: 1, costPerUnit: 100 },
    ],
    strategies: [
      { id: 1, name: 'Стратегия 1: Агрессивный маркетинг', description: 'Форсированный захват рынка (+15 Доля рынка, -5 Качество)', cost: 15, active: false },
      { id: 2, name: 'Стратегия 2: Безупречное качество', description: 'Фокус на стабильности и надежности (+15 Качество, +8 Доверие)', cost: 15, active: false },
      { id: 3, name: 'Стратегия 3: Корпоративные продажи B2B', description: 'Работа с крупными клиентами (+12 Доля рынка, +10 Доверие)', cost: 20, active: false },
    ],
    hints: [
      { id: 20, name: 'Исследование 1: Интервью целевой аудитории', description: 'Выявление скрытых потребностей клиентов', reliability: 'высокая', cost: 4, used: false, secret: 'Клиенты готовы доплачивать 20% за интеграцию с популярными CRM-системами.' },
      { id: 21, name: 'Исследование 2: Аудит конкурентов', description: 'Анализ тарифов и слабых мест конкурентов', reliability: 'средняя', cost: 4, used: false, secret: 'Главный конкурент страдает от частых сбоев в выходные дни.' },
      { id: 22, name: 'Исследование 3: Оценка технического долга', description: 'Аудит архитектуры базы данных', reliability: 'высокая', cost: 5, used: false, secret: 'Оптимизация индексов базы данных сократит расходы на сервера в 2 раза.' },
    ],
    history: [],
  };
}

// Изолированные состояния для каждой команды
let teamStates = {
  1: createInitialTeamState(1),
  2: createInitialTeamState(2),
};

// Индивидуальные предсказания событий для каждой команды
let teamPredictions = {
  1: [],
  2: [],
};

let decisions = {};
let grades = {};

// Генерация уникальных предсказаний для команды
function generatePredictionsForTeam(teamId, round) {
  // Разные наборы событий для разных команд
  const team1EventIds = round === 1 ? [101, 102, 103] : [104, 107, 109];
  const team2EventIds = round === 1 ? [105, 106, 107] : [101, 108, 110];

  const pool = (teamId % 2 === 1) ? team1EventIds : team2EventIds;

  return pool.map((eventId, idx) => {
    const raw = EVENT_CATALOG[eventId];
    const prob = idx === 0 ? 0.75 : idx === 1 ? 0.45 : 0.30;
    return {
      event: {
        id: raw.id,
        name: raw.name,
        description: raw.description,
        imageUrl: raw.imageUrl,
        isPositive: raw.isPositive,
      },
      probability: prob,
      suggested: idx === 0, // первое событие по умолчанию рекомендовано
    };
  });
}

// --- WebSocket соединения ---
const clients = new Set(); // { ws, token, isAdmin, teamId }

function broadcast(type, payload) {
  const msg = JSON.stringify({ type, payload });
  for (const client of clients) {
    if (client.ws.readyState === WebSocket.OPEN) {
      try {
        client.ws.send(msg);
      } catch (e) {
        console.error('[WS] Send error:', e);
      }
    }
  }
}

function broadcastToTeam(teamId, type, payload) {
  const msg = JSON.stringify({ type, payload });
  for (const client of clients) {
    if (client.teamId === teamId && client.ws.readyState === WebSocket.OPEN) {
      try {
        client.ws.send(msg);
      } catch (e) {
        console.error('[WS] Send error:', e);
      }
    }
  }
}

function sendJson(res, status, data) {
  res.writeHead(status, {
    'Content-Type': 'application/json',
    'Access-Control-Allow-Origin': '*',
    'Access-Control-Allow-Headers': 'Content-Type, Authorization',
    'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS',
  });
  res.end(JSON.stringify(data));
}

function parseBody(req) {
  return new Promise((resolve) => {
    let body = '';
    req.on('data', (chunk) => { body += chunk; });
    req.on('end', () => {
      try {
        resolve(body ? JSON.parse(body) : {});
      } catch {
        resolve({});
      }
    });
  });
}

const server = http.createServer(async (req, res) => {
  const parsedUrl = url.parse(req.url, true);
  const path = parsedUrl.pathname;
  const method = req.method;

  if (method === 'OPTIONS') {
    res.writeHead(204, {
      'Access-Control-Allow-Origin': '*',
      'Access-Control-Allow-Headers': 'Content-Type, Authorization',
      'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS',
    });
    res.end();
    return;
  }

  // --- GET /api/game ---
  if (method === 'GET' && path === '/api/game') {
    return sendJson(res, 200, gameState);
  }

  // --- POST /api/teams (Регистрация команды) ---
  if (method === 'POST' && path === '/api/teams') {
    const body = await parseBody(req);
    const newId = teams.length + 1;
    const newTeam = {
      id: newId,
      name: body.name || `Команда ${newId}`,
      members: body.members || ['Участник 1'],
      ready: typeof body.ready === 'boolean' ? body.ready : false,
      online: true,
      submitted: false,
      bankrupt: false,
      token: `token-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
    };
    teams.push(newTeam);
    teamStates[newId] = createInitialTeamState(newId);
    teamPredictions[newId] = [];

    broadcast('lobby_updated', { teams });
    return sendJson(res, 201, { token: newTeam.token, team: newTeam });
  }

  // --- PUT /api/team/ready (Готовность) ---
  if (method === 'PUT' && path === '/api/team/ready') {
    const body = await parseBody(req);
    const authHeader = req.headers.authorization || '';
    const token = authHeader.replace(/^Bearer\s+/, '').trim();
    const team = teams.find((t) => t.token === token) || teams[0];
    if (team) {
      team.ready = typeof body.ready === 'boolean' ? body.ready : !team.ready;
      broadcast('lobby_updated', { teams });
      return sendJson(res, 200, team);
    }
    return sendJson(res, 404, { error: { code: 'not_found', message: 'Team not found' } });
  }

  // --- GET /api/team/state ---
  if (method === 'GET' && path === '/api/team/state') {
    const authHeader = req.headers.authorization || '';
    const token = authHeader.replace(/^Bearer\s+/, '').trim();
    const currentTeam = teams.find((t) => t.token === token) || teams[0];

    if (!currentTeam) {
      return sendJson(res, 401, { error: { code: 'unauthorized', message: 'Unauthorized' } });
    }

    const tState = teamStates[currentTeam.id] || createInitialTeamState(currentTeam.id);

    return sendJson(res, 200, {
      game: gameState,
      team: currentTeam,
      opening: {
        id: 0,
        name: 'Старт симуляции: Первые шаги компании',
        description: 'Ваша команда руководит развитием технологического стартапа. Распределяйте бюджет, запускайте стратегии и исследуйте рынок!',
        imageUrl: '',
        isPositive: true,
      },
      state: gameState.phase === 'lobby' ? null : tState,
      decision: decisions[currentTeam.id] || null,
    });
  }

  // --- POST /api/team/decision (Отправка решения) ---
  if (method === 'POST' && path === '/api/team/decision') {
    const body = await parseBody(req);
    const authHeader = req.headers.authorization || '';
    const token = authHeader.replace(/^Bearer\s+/, '').trim();
    const currentTeam = teams.find((t) => t.token === token) || teams[0];

    if (currentTeam) {
      decisions[currentTeam.id] = body;
      currentTeam.submitted = true;

      const tState = teamStates[currentTeam.id];
      let moneyCost = gameState.baseRoundCost;
      if (body.attributes && tState) {
        body.attributes.forEach((a) => {
          const cfg = tState.configurable.find((x) => x.id === a.id);
          if (cfg) moneyCost += Math.round(a.value * cfg.costPerUnit);
        });
      }
      if (body.strategies && tState) {
        body.strategies.forEach((sid) => {
          const strat = tState.strategies.find((x) => x.id === sid);
          if (strat) moneyCost += strat.cost;
        });
      }

      let qpCost = 0;
      if (body.hints && tState) {
        body.hints.forEach((hid) => {
          const hint = tState.hints.find((x) => x.id === hid);
          if (hint) qpCost += hint.cost;
        });
      }

      broadcast('submission_updated', { teamId: currentTeam.id, submitted: true });
      return sendJson(res, 200, { cost: { money: moneyCost, qp: qpCost } });
    }
    return sendJson(res, 404, { error: { code: 'not_found', message: 'Team not found' } });
  }

  // --- GET /api/admin/scenarios ---
  if (method === 'GET' && path === '/api/admin/scenarios') {
    return sendJson(res, 200, { scenarios: [{ id: 'demo' }, { id: 'it-startup' }, { id: 'market-crisis' }] });
  }

  // --- POST /api/admin/game (Создать игру) ---
  if (method === 'POST' && path === '/api/admin/game') {
    gameState = {
      gameId: gameState.gameId + 1,
      phase: 'lobby',
      round: 0,
      scenarioId: 'demo',
      baseRoundCost: 5,
    };
    decisions = {};
    teams = [
      {
        id: 1,
        name: 'Альфа',
        members: ['Алексей Иванов', 'Мария Смирнова'],
        ready: false,
        online: false,
        submitted: false,
        bankrupt: false,
        token: 'token-alpha-123',
      },
      {
        id: 2,
        name: 'Бета',
        members: ['Дмитрий Кузнецов', 'Елена Попова'],
        ready: true,
        online: false,
        submitted: false,
        bankrupt: false,
        token: 'token-beta-456',
      },
    ];
    teamStates = {};
    teamPredictions = {};
    teams.forEach((t) => {
      teamStates[t.id] = createInitialTeamState(t.id);
      teamPredictions[t.id] = [];
    });

    broadcast('phase_changed', gameState);
    broadcast('lobby_updated', { teams });

    return sendJson(res, 201, { game: gameState, teams });
  }

  // --- POST /api/admin/game/start (Старт игры) ---
  if (method === 'POST' && path === '/api/admin/game/start') {
    const body = await parseBody(req);
    gameState.phase = 'round_open';
    gameState.round = 1;
    gameState.scenarioId = body.scenarioId || 'demo';
    teams.forEach((t) => {
      t.submitted = false;
      if (!teamStates[t.id]) teamStates[t.id] = createInitialTeamState(t.id);
    });
    decisions = {};

    broadcast('phase_changed', gameState);
    return sendJson(res, 200, { game: gameState, teams });
  }

  // --- DELETE /api/admin/teams/:teamId ---
  if (method === 'DELETE' && path.startsWith('/api/admin/teams/')) {
    const parts = path.split('/');
    const teamId = parseInt(parts[parts.length - 1]);
    teams = teams.filter((t) => t.id !== teamId);
    delete teamStates[teamId];
    delete teamPredictions[teamId];
    delete decisions[teamId];

    broadcast('lobby_updated', { teams });
    return sendJson(res, 200, { game: gameState, teams });
  }

  // --- GET /api/admin/state ---
  if (method === 'GET' && path === '/api/admin/state') {
    const adminTeams = teams.map((t) => {
      const tState = teamStates[t.id] || createInitialTeamState(t.id);
      return {
        ...t,
        state: tState,
        decision: decisions[t.id] || null,
        cost: decisions[t.id] ? { money: 35, qp: 4 } : null,
        predictions: gameState.phase === 'round_closed' ? (teamPredictions[t.id] || []) : [],
      };
    });
    return sendJson(res, 200, { game: gameState, teams: adminTeams });
  }

  // --- POST /api/admin/round/close (Завершить сбор решений) ---
  if (method === 'POST' && path === '/api/admin/round/close') {
    gameState.phase = 'round_closed';

    // Для КАЖДОЙ команды генерируем индивидуальные предсказания
    teams.forEach((t) => {
      teamPredictions[t.id] = generatePredictionsForTeam(t.id, gameState.round);
    });

    broadcast('phase_changed', gameState);

    const adminTeams = teams.map((t) => ({
      ...t,
      state: teamStates[t.id],
      decision: decisions[t.id] || null,
      cost: decisions[t.id] ? { money: 35, qp: 4 } : null,
      predictions: teamPredictions[t.id] || [],
    }));
    return sendJson(res, 200, { game: gameState, teams: adminTeams });
  }

  // --- POST /api/admin/round/events (Применить события) ---
  if (method === 'POST' && path === '/api/admin/round/events') {
    const body = await parseBody(req);
    const selections = body.selections || [];

    if (body.finish) {
      gameState.phase = 'finished';
      broadcast('phase_changed', gameState);
    } else {
      // ИНДИВИДУАЛЬНО ДЛЯ КАЖДОЙ КОМАНДЫ применяем события и пересчитываем показатели
      for (const sel of selections) {
        const teamId = sel.teamId;
        const tState = teamStates[teamId];
        if (!tState) continue;

        const currentPredictions = teamPredictions[teamId] || [];
        const selectedEventIds = sel.eventIds || [];

        // Только события, выбранные для ЭТОЙ команды
        const eventsFired = selectedEventIds
          .map((id) => {
            const fromPred = currentPredictions.find((p) => p.event.id === id);
            return fromPred ? fromPred.event : EVENT_CATALOG[id];
          })
          .filter(Boolean);

        // Сохраняем в историю ТОЛЬКО этой команды
        tState.history.push({
          round: gameState.round,
          events: eventsFired,
        });

        // 1. Применяем решения команды (слайдеры, стратегии, исследования)
        const dec = decisions[teamId];
        let sliderCost = 0;
        let stratCost = 0;
        let qpSpent = 0;

        if (dec) {
          // Обновляем слайдеры
          if (dec.attributes) {
            dec.attributes.forEach((a) => {
              const cfg = tState.configurable.find((x) => x.id === a.id);
              if (cfg) {
                cfg.value = a.value;
                sliderCost += Math.round(a.value * cfg.costPerUnit);
              }
            });
          }

          // Обновляем стратегии
          if (dec.strategies) {
            tState.strategies.forEach((s) => {
              s.active = dec.strategies.includes(s.id);
              if (s.active) stratCost += s.cost;
            });
          }

          // Разблокируем купленные подсказки
          if (dec.hints) {
            dec.hints.forEach((hid) => {
              const h = tState.hints.find((x) => x.id === hid);
              if (h) {
                h.used = true;
                qpSpent += h.cost;
              }
            });
          }
        }

        // 2. Считаем влияние решений на показатели
        const marketing = tState.configurable.find((c) => c.id === 10)?.value || 0.3;
        const dev = tState.configurable.find((c) => c.id === 11)?.value || 0.3;
        const sales = tState.configurable.find((c) => c.id === 12)?.value || 0.2;
        const rd = tState.configurable.find((c) => c.id === 13)?.value || 0.2;

        let deltaTrust = Math.round(marketing * 8 + dev * 6);
        let deltaProduct = Math.round(dev * 16 + rd * 8);
        let deltaTeam = Math.round(dev * 4 + rd * 4);
        let deltaMarket = Math.round(marketing * 14 + sales * 12);

        // Влияние активных стратегий
        if (tState.strategies.find((s) => s.id === 1 && s.active)) {
          deltaMarket += 12;
          deltaProduct -= 4;
        }
        if (tState.strategies.find((s) => s.id === 2 && s.active)) {
          deltaProduct += 12;
          deltaTrust += 8;
        }
        if (tState.strategies.find((s) => s.id === 3 && s.active)) {
          deltaMarket += 10;
          deltaTrust += 6;
        }

        // 3. Влияние выбранных событий
        let eventMoney = 0;
        let eventQP = 0;

        for (const ev of eventsFired) {
          const raw = EVENT_CATALOG[ev.id] || ev;
          if (raw) {
            eventMoney += raw.money || 0;
            eventQP += raw.qp || 0;
            if (raw.effects) {
              if (raw.effects[1]) deltaTrust += raw.effects[1];
              if (raw.effects[2]) deltaProduct += raw.effects[2];
              if (raw.effects[3]) deltaTeam += raw.effects[3];
              if (raw.effects[4]) deltaMarket += raw.effects[4];
            }
          }
        }

        // 4. Применяем изменения к показателям (ограничиваем 5..100)
        tState.attributes.forEach((attr) => {
          if (attr.id === 1) attr.value = Math.min(100, Math.max(5, attr.value + deltaTrust));
          if (attr.id === 2) attr.value = Math.min(100, Math.max(5, attr.value + deltaProduct));
          if (attr.id === 3) attr.value = Math.min(100, Math.max(5, attr.value + deltaTeam));
          if (attr.id === 4) attr.value = Math.min(100, Math.max(5, attr.value + deltaMarket));
        });

        // 5. Пересчет бюджета (Доходы от доли рынка и продукта - Расходы + События)
        const marketShare = tState.attributes.find((a) => a.id === 4)?.value || 30;
        const productQuality = tState.attributes.find((a) => a.id === 2)?.value || 40;
        const revenue = Math.round(25 + (marketShare * 0.4) + (productQuality * 0.2));

        const totalDecisionCost = gameState.baseRoundCost + sliderCost + stratCost;
        tState.budget.money = Math.max(0, tState.budget.money - totalDecisionCost + revenue + eventMoney);
        tState.budget.qp = Math.max(0, tState.budget.qp - qpSpent + eventQP);

        // Отправляем личное событие КОНКРЕТНОЙ команде
        broadcastToTeam(teamId, 'team_state', {
          round: gameState.round + 1,
          events: eventsFired,
        });
      }

      gameState.phase = 'round_open';
      gameState.round += 1;
      teams.forEach((t) => { t.submitted = false; });
      decisions = {};
      teamPredictions = {};

      broadcast('phase_changed', gameState);
    }

    const adminTeams = teams.map((t) => ({
      ...t,
      state: teamStates[t.id],
      decision: null,
      cost: null,
      predictions: [],
    }));
    return sendJson(res, 200, { game: gameState, teams: adminTeams });
  }

  // --- POST /api/admin/game/finish ---
  if (method === 'POST' && path === '/api/admin/game/finish') {
    gameState.phase = 'finished';
    broadcast('phase_changed', gameState);
    return sendJson(res, 200, { game: gameState, teams });
  }

  // --- GET /api/admin/games/:id ---
  if (method === 'GET' && path.startsWith('/api/admin/games/')) {
    return sendJson(res, 200, {
      game: {
        gameId: gameState.gameId,
        scenarioId: gameState.scenarioId,
        phase: gameState.phase,
        rounds: gameState.round,
        active: true,
        createdAt: new Date().toISOString(),
      },
      teams: teams.map((t) => ({
        id: t.id,
        name: t.name,
        members: t.members,
        rounds: [],
        grades: grades[t.id] || [],
      })),
    });
  }

  // --- PUT /api/admin/games/:gameId/teams/:teamId/grades ---
  if (method === 'PUT' && path.includes('/grades')) {
    const body = await parseBody(req);
    const parts = path.split('/');
    const teamId = parts[parts.indexOf('teams') + 1];
    grades[teamId] = body.grades || [];
    return sendJson(res, 200, body);
  }

  // 404
  sendJson(res, 404, { error: { code: 'not_found', message: `Route not found: ${method} ${path}` } });
});

// --- WebSocket Server ---
const wss = new WebSocketServer({ server });

wss.on('connection', (ws, req) => {
  const parsedUrl = url.parse(req.url, true);
  const token = parsedUrl.query.token;

  let teamId = null;
  const isAdmin = token === 'admin' || token === 'admin-token';
  const team = teams.find((t) => t.token === token);
  if (team) {
    teamId = team.id;
    team.online = true;
    broadcast('lobby_updated', { teams });
  }

  const client = { ws, token, isAdmin, teamId };
  clients.add(client);
  console.log(`[WS] Client connected (token: ${token || 'guest'}, teamId: ${teamId})`);

  ws.on('close', () => {
    clients.delete(client);
    if (team) {
      const hasOther = Array.from(clients).some((c) => c.teamId === teamId);
      if (!hasOther) {
        team.online = false;
        broadcast('team_online', { teamId, online: false });
        broadcast('lobby_updated', { teams });
      }
    }
    console.log(`[WS] Client disconnected`);
  });
});

const PORT = 8080;
server.listen(PORT, () => {
  console.log(`[Mock Server] Запущен на http://localhost:${PORT}`);
  console.log(`[Mock Server] WebSocket активен на ws://localhost:${PORT}/ws`);
  console.log(`[Mock Server] Мультиплеер, динамическая симуляция и изоляция команд активны`);
});
