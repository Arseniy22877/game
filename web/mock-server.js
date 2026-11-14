// Простой локальный мок-сервер бэкенда для тестирования фронтенда (порт 8080)
// Запуск: node mock-server.js

import http from 'http';
import url from 'url';

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
    members: ['Алексей', 'Мария'],
    ready: true,
    online: true,
    submitted: false,
    bankrupt: false,
    token: 'token-alpha-123',
  },
];

let decisions = {};
let grades = {};

const defaultTeamGameState = {
  budget: { money: 100, maxMoney: 150, qp: 10 },
  attributes: [
    { id: 0, name: 'Доверие рынка', description: 'Уровень доверия клиентов', value: 0.65, min: 0, max: 1 },
    { id: 1, name: 'Стабильность', description: 'Надёжность инфраструктуры', value: 0.75, min: 0, max: 1 },
  ],
  configurable: [
    { id: 10, name: 'Маркетинг', description: 'Привлечение пользователей', value: 0.3, min: 0, max: 1, costPerUnit: 50 },
    { id: 11, name: 'Разработка', description: 'Улучшение функций продукта', value: 0.4, min: 0, max: 1, costPerUnit: 60 },
  ],
  strategies: [
    { id: 1, name: 'Агрессивный рост', description: 'Увеличение доли рынка любой ценой', cost: 20, active: false },
    { id: 2, name: 'Фокус на качестве', description: 'Повышение стабильности системы', cost: 15, active: false },
  ],
  hints: [
    { id: 20, name: 'Опрос пользователей', description: 'Узнать требования целевой аудитории', reliability: 'высокая', cost: 3, used: false, secret: 'Пользователи жалуются на интерфейс' },
    { id: 21, name: 'Анализ конкурентов', description: 'Изучить стратегию лидеров рынка', reliability: 'средняя', cost: 4, used: false, secret: 'Конкуренты снижают цены на подписку' },
  ],
  history: [],
};

const predictions = [
  {
    event: { id: 101, name: 'Приток инвестиций', description: 'Инвесторы выделили дополнительный грант', imageUrl: '', isPositive: true },
    probability: 0.72,
    suggested: true,
  },
  {
    event: { id: 102, name: 'Сбой серверов', description: 'В датацентре произошла авария', imageUrl: '', isPositive: false },
    probability: 0.35,
    suggested: false,
  },
];

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

  // --- POST /api/teams ---
  if (method === 'POST' && path === '/api/teams') {
    const body = await parseBody(req);
    const newTeam = {
      id: teams.length + 1,
      name: body.name || `Команда ${teams.length + 1}`,
      members: body.members || ['Игрок 1'],
      ready: true,
      online: true,
      submitted: false,
      bankrupt: false,
      token: `token-${Date.now()}`,
    };
    teams.push(newTeam);
    return sendJson(res, 201, { token: newTeam.token, team: newTeam });
  }

  // --- PUT /api/team/ready ---
  if (method === 'PUT' && path === '/api/team/ready') {
    const body = await parseBody(req);
    if (teams[0]) teams[0].ready = body.ready ?? !teams[0].ready;
    return sendJson(res, 200, teams[0] || {});
  }

  // --- GET /api/team/state ---
  if (method === 'GET' && path === '/api/team/state') {
    const currentTeam = teams[0] || {
      id: 1, name: 'Демо-команда', members: ['Игрок'], ready: true, online: true, submitted: false, bankrupt: false,
    };
    return sendJson(res, 200, {
      game: gameState,
      team: currentTeam,
      opening: gameState.round === 1 ? {
        id: 0,
        name: 'Начало бизнес-симуляции',
        description: 'Ваша компания выходит на новый рынок. Грамотно распределяйте бюджет между разработкой и маркетингом!',
        imageUrl: '',
        isPositive: true,
      } : null,
      state: gameState.phase === 'lobby' ? null : defaultTeamGameState,
      decision: decisions[currentTeam.id] || null,
    });
  }

  // --- POST /api/team/decision ---
  if (method === 'POST' && path === '/api/team/decision') {
    const body = await parseBody(req);
    decisions[1] = body;
    if (teams[0]) teams[0].submitted = true;
    return sendJson(res, 200, { cost: { money: 35, qp: 3 } });
  }

  // --- GET /api/admin/scenarios ---
  if (method === 'GET' && path === '/api/admin/scenarios') {
    return sendJson(res, 200, { scenarios: [{ id: 'demo' }, { id: 'it-startup' }] });
  }

  // --- POST /api/admin/game ---
  if (method === 'POST' && path === '/api/admin/game') {
    gameState = {
      gameId: gameState.gameId + 1,
      phase: 'lobby',
      round: 0,
      scenarioId: 'demo',
      baseRoundCost: 5,
    };
    decisions = {};
    return sendJson(res, 201, { game: gameState, teams });
  }

  // --- POST /api/admin/game/start ---
  if (method === 'POST' && path === '/api/admin/game/start') {
    const body = await parseBody(req);
    gameState.phase = 'round_open';
    gameState.round = 1;
    gameState.scenarioId = body.scenarioId || 'demo';
    teams.forEach((t) => { t.submitted = false; });
    return sendJson(res, 200, { game: gameState, teams });
  }

  // --- GET /api/admin/state ---
  if (method === 'GET' && path === '/api/admin/state') {
    const adminTeams = teams.map((t) => ({
      ...t,
      state: defaultTeamGameState,
      decision: decisions[t.id] || null,
      cost: decisions[t.id] ? { money: 35, qp: 3 } : null,
      predictions: gameState.phase === 'round_closed' ? predictions : [],
    }));
    return sendJson(res, 200, { game: gameState, teams: adminTeams });
  }

  // --- POST /api/admin/round/close ---
  if (method === 'POST' && path === '/api/admin/round/close') {
    gameState.phase = 'round_closed';
    const adminTeams = teams.map((t) => ({
      ...t,
      state: defaultTeamGameState,
      decision: decisions[t.id] || null,
      cost: { money: 35, qp: 3 },
      predictions,
    }));
    return sendJson(res, 200, { game: gameState, teams: adminTeams });
  }

  // --- POST /api/admin/round/events ---
  if (method === 'POST' && path === '/api/admin/round/events') {
    const body = await parseBody(req);
    if (body.finish) {
      gameState.phase = 'finished';
    } else {
      gameState.phase = 'round_open';
      gameState.round += 1;
      teams.forEach((t) => { t.submitted = false; });
      decisions = {};
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
    return sendJson(res, 200, { game: gameState, teams });
  }

  // --- GET /api/admin/games/:id ---
  if (method === 'GET' && path.startsWith('/api/admin/games/')) {
    return sendJson(res, 200, {
      game: { gameId: gameState.gameId, scenarioId: gameState.scenarioId, phase: gameState.phase, rounds: gameState.round, active: true, createdAt: new Date().toISOString() },
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

const PORT = 8080;
server.listen(PORT, () => {
  console.log(`[Mock Server] Запущен на http://localhost:${PORT}`);
  console.log(`[Mock Server] Эмулирует Go-бэкенд для тестирования страниц фронтенда`);
});
