// Простой локальный мок-сервер бэкенда для тестирования фронтенда (порт 8080)
// Включает полную поддержку REST API и WebSocket с бродкастом всех событий в реальном времени.
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
    ready: true,
    online: false,
    submitted: false,
    bankrupt: false,
    token: 'token-alpha-123',
  },
  {
    id: 2,
    name: 'Бета',
    members: ['Дмитрий Кузнецов', 'Елена Попова'],
    ready: false,
    online: false,
    submitted: false,
    bankrupt: false,
    token: 'token-beta-456',
  },
];

let decisions = {};
let grades = {};

const defaultTeamGameState = {
  budget: { money: 100, maxMoney: 150, qp: 10 },
  attributes: [
    { id: 1, name: 'Показатель 1 (Доверие)', description: 'Доверие клиентов и партнеров', value: 56, min: 0, max: 100 },
    { id: 2, name: 'Показатель 2 (Продукт)', description: 'Качество и стабильность продукта', value: 32, min: 0, max: 100 },
    { id: 3, name: 'Показатель 3 (Команда)', description: 'Мотивация и продуктивность сотрудников', value: 68, min: 0, max: 100 },
    { id: 4, name: 'Показатель 4 (Доля рынка)', description: 'Охват рынка и узнаваемость', value: 94, min: 0, max: 100 },
  ],
  configurable: [
    { id: 10, name: 'Сфера 1: Маркетинг', description: 'Привлечение новых клиентов и реклама', value: 0.32, min: 0, max: 1, costPerUnit: 100 },
    { id: 11, name: 'Сфера 2: Разработка', description: 'Развитие ключевых функций платформы', value: 0.32, min: 0, max: 1, costPerUnit: 100 },
    { id: 12, name: 'Сфера 3: Продажи', description: 'Расширение каналов продаж и клиентский сервис', value: 0.32, min: 0, max: 1, costPerUnit: 100 },
    { id: 13, name: 'Сфера 4: Исследования', description: 'Анализ пользовательского опыта и R&D', value: 0.32, min: 0, max: 1, costPerUnit: 100 },
  ],
  strategies: [
    {
      id: 1,
      name: 'Стратегия 1: Агрессивный рост',
      description: 'Общий долгосрочный план действий, направленный на достижение сложной глобальной цели в условиях ограниченности ресурсов и неопределенности.',
      cost: 10,
      active: false,
    },
    {
      id: 2,
      name: 'Стратегия 2: Оптимизация расходов',
      description: 'Снижение операционных издержек и фокусировка на удержании существующих клиентов.',
      cost: 10,
      active: false,
    },
    {
      id: 3,
      name: 'Стратегия 3: Выход в премиум-сегмент',
      description: 'Позиционирование продукта для корпоративных заказчиков с высоким чеком.',
      cost: 10,
      active: false,
    },
  ],
  hints: [
    {
      id: 20,
      name: 'Исследование 1: Глубинные интервью клиентов',
      description: 'Исследование целевой аудитории для выявления ключевых барьеров использования.',
      reliability: 'высокая',
      cost: 10,
      used: false,
      secret: 'Клиенты готовы платить на 25% больше за круглосуточную поддержку.',
    },
    {
      id: 21,
      name: 'Исследование 2: Анализ ценовой политики конкурентов',
      description: 'Исследование рынка и тарифных сеток основных игроков.',
      reliability: 'средняя',
      cost: 10,
      used: false,
      secret: 'Главный конкурент готовит демпинг в следующем квартале.',
    },
    {
      id: 22,
      name: 'Исследование 3: Аудит технической архитектуры',
      description: 'Оценка надежности серверной инфраструктуры при пиковых нагрузках.',
      reliability: 'высокая',
      cost: 10,
      used: true,
      secret: 'Обнаружены узкие места в базе данных, нужен рефакторинг.',
    },
  ],
  history: [
    {
      round: 0,
      events: [
        {
          id: 0,
          name: 'Успешный запуск проекта',
          description: 'Проект стартовал без инцидентов. Рынок встретил продукт с умеренным оптимизмом.',
          imageUrl: '',
          isPositive: true,
        },
      ],
    },
  ],
};

const predictions = [
  {
    event: {
      id: 101,
      name: 'Интерес венчурного фонда',
      description: 'Крупный инвестор предлагает раунд финансирования на выгодных условиях.',
      imageUrl: '',
      isPositive: true,
    },
    probability: 0.75,
    suggested: true,
  },
  {
    event: {
      id: 102,
      name: 'Атака конкурентов на репутацию',
      description: 'В отраслевых СМИ появилась заказная статья с критикой безопасности сервиса.',
      imageUrl: '',
      isPositive: false,
    },
    probability: 0.45,
    suggested: false,
  },
  {
    event: {
      id: 103,
      name: 'Хакатон инноваций',
      description: 'Команда инженеров за выходные создала прототип востребованной пользователями фичи.',
      imageUrl: '',
      isPositive: true,
    },
    probability: 0.30,
    suggested: false,
  },
];

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
    const newTeam = {
      id: teams.length + 1,
      name: body.name || `Команда ${teams.length + 1}`,
      members: body.members || ['Участник 1'],
      ready: true,
      online: true,
      submitted: false,
      bankrupt: false,
      token: `token-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
    };
    teams.push(newTeam);

    // Уведомляем препода и команды об обновлении лобби
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
      team.ready = body.ready !== undefined ? body.ready : !team.ready;
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

    return sendJson(res, 200, {
      game: gameState,
      team: currentTeam,
      opening: {
        id: 0,
        name: 'Старт симуляции: Первые шаги компании',
        description: 'Ваша команда руководит развитием нового технологического стартапа. Распределяйте бюджет по ключевым направлениям, запускайте стратегии и исследуйте рынок!',
        imageUrl: '',
        isPositive: true,
      },
      state: gameState.phase === 'lobby' ? null : defaultTeamGameState,
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

      // Оповещаем препода и участников о сдаче
      broadcast('submission_updated', { teamId: currentTeam.id, submitted: true });

      return sendJson(res, 200, { cost: { money: 35, qp: 10 } });
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
    teams.forEach((t) => { t.submitted = false; });

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
    teams.forEach((t) => { t.submitted = false; });
    decisions = {};

    broadcast('phase_changed', gameState);

    return sendJson(res, 200, { game: gameState, teams });
  }

  // --- DELETE /api/admin/teams/:teamId ---
  if (method === 'DELETE' && path.startsWith('/api/admin/teams/')) {
    const parts = path.split('/');
    const teamId = parseInt(parts[parts.length - 1]);
    teams = teams.filter((t) => t.id !== teamId);
    broadcast('lobby_updated', { teams });
    return sendJson(res, 200, { game: gameState, teams });
  }

  // --- GET /api/admin/state ---
  if (method === 'GET' && path === '/api/admin/state') {
    const adminTeams = teams.map((t) => ({
      ...t,
      state: defaultTeamGameState,
      decision: decisions[t.id] || null,
      cost: decisions[t.id] ? { money: 35, qp: 10 } : null,
      predictions: gameState.phase === 'round_closed' ? predictions : [],
    }));
    return sendJson(res, 200, { game: gameState, teams: adminTeams });
  }

  // --- POST /api/admin/round/close (Завершить сбор решений) ---
  if (method === 'POST' && path === '/api/admin/round/close') {
    gameState.phase = 'round_closed';

    broadcast('phase_changed', gameState);

    const adminTeams = teams.map((t) => ({
      ...t,
      state: defaultTeamGameState,
      decision: decisions[t.id] || null,
      cost: decisions[t.id] ? { money: 35, qp: 10 } : null,
      predictions,
    }));
    return sendJson(res, 200, { game: gameState, teams: adminTeams });
  }

  // --- POST /api/admin/round/events (Применить события) ---
  if (method === 'POST' && path === '/api/admin/round/events') {
    const body = await parseBody(req);
    if (body.finish) {
      gameState.phase = 'finished';
      broadcast('phase_changed', gameState);
    } else {
      // Для каждой команды отправляем произошедшие события и сохраняем в историю
      for (const sel of (body.selections || [])) {
        const eventsFired = predictions
          .filter((p) => sel.eventIds.includes(p.event.id))
          .map((p) => p.event);

        defaultTeamGameState.history.push({
          round: gameState.round,
          events: eventsFired,
        });

        broadcastToTeam(sel.teamId, 'team_state', {
          round: gameState.round + 1,
          events: eventsFired,
        });
      }

      // Все купленные подсказки открывают результат
      defaultTeamGameState.hints.forEach((h) => {
        h.used = true;
      });

      gameState.phase = 'round_open';
      gameState.round += 1;
      teams.forEach((t) => { t.submitted = false; });
      decisions = {};

      broadcast('phase_changed', gameState);
    }

    const adminTeams = teams.map((t) => ({
      ...t,
      state: defaultTeamGameState,
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

// --- Настройка WebSocket Server ---
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
      // Проверяем, есть ли ещё активные соединения этой команды
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
  console.log(`[Mock Server] Полный реалтайм и бродкаст событий включены`);
});
