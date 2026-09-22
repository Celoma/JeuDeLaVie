(() => {
  const canvas = document.querySelector('#map-canvas');
  const canvasWrap = document.querySelector('#canvas-wrap');
  const loadingCard = document.querySelector('#loading-card');
  const loadingMessage = document.querySelector('#loading-message');
  const status = document.querySelector('#status');
  const statusLabel = document.querySelector('#status-label');
  const zoomValue = document.querySelector('#zoom-value');
  const coordinates = document.querySelector('#coordinates');
  const context = canvas.getContext('2d');

  const view = { scale: 1, offsetX: 0, offsetY: 0, dragging: false, pointerX: 0, pointerY: 0 };
  let map = null;
  let simulationTick = 0;
  let simulationRunning = false;
  let tickInFlight = false;
  let runToken = 0;
  let pendingSimulationStats = null;
  let simulationRenderScheduled = false;
  let devicePixelRatio = window.devicePixelRatio || 1;

  const colors = { background: '#e6efeb', grid: '#cbded8', healthy: '#65b8aa', infected: '#e76552', city: '#dfa83c', igloo: '#76a3ae' };

  function setText(selector, value) { document.querySelector(selector).textContent = value; }

  function resizeCanvas() {
    const bounds = canvasWrap.getBoundingClientRect();
    devicePixelRatio = window.devicePixelRatio || 1;
    canvas.width = Math.max(1, Math.floor(bounds.width * devicePixelRatio));
    canvas.height = Math.max(1, Math.floor(bounds.height * devicePixelRatio));
    context.setTransform(devicePixelRatio, 0, 0, devicePixelRatio, 0, 0);
    draw();
  }

  function fitMap() {
    if (!map) return;
    const bounds = canvasWrap.getBoundingClientRect();
    const padding = 42;
    view.scale = Math.min((bounds.width - padding * 2) / map.width, (bounds.height - padding * 2) / map.height);
    view.offsetX = (bounds.width - map.width * view.scale) / 2;
    view.offsetY = (bounds.height - map.height * view.scale) / 2;
    draw();
  }

  function drawGrid(width, height) {
    const step = 100 * view.scale;
    if (step < 8) return;
    context.strokeStyle = colors.grid;
    context.lineWidth = 1;
    context.beginPath();
    const startX = view.offsetX % step;
    const startY = view.offsetY % step;
    for (let x = startX; x < width; x += step) { context.moveTo(x, 0); context.lineTo(x, height); }
    for (let y = startY; y < height; y += step) { context.moveTo(0, y); context.lineTo(width, y); }
    context.stroke();
  }

  function drawPeople() {
    const radius = Math.max(1.25, Math.min(3.2, view.scale * 1.75));
    for (const person of map.people) {
      context.beginPath();
      context.fillStyle = person.dead ? '#172126' : person.immune ? '#d5a83e' : person.infected ? colors.infected : colors.healthy;
      context.arc(view.offsetX + person.x * view.scale, view.offsetY + person.y * view.scale, person.infected ? radius + 1.4 : radius, 0, Math.PI * 2);
      context.fill();
    }
  }

  function draw() {
    if (!map) return;
    const bounds = canvasWrap.getBoundingClientRect();
    context.clearRect(0, 0, bounds.width, bounds.height);
    context.fillStyle = colors.background;
    context.fillRect(0, 0, bounds.width, bounds.height);
    drawGrid(bounds.width, bounds.height);
    context.save();
    context.beginPath();
    context.rect(view.offsetX, view.offsetY, map.width * view.scale, map.height * view.scale);
    context.clip();
    drawPeople();
    context.restore();
    context.strokeStyle = '#a6c6be';
    context.lineWidth = 1;
    context.strokeRect(view.offsetX, view.offsetY, map.width * view.scale, map.height * view.scale);
    zoomValue.textContent = `${Math.round(view.scale * 100)}%`;
  }

  function updateStats() {
    const infected = map.people.filter((person) => person.infected).length;
    setText('#map-size', `${map.width} × ${map.height}`);
    setText('#people-count', map.people.length.toLocaleString('fr-FR'));
    setText('#infected-count', infected.toLocaleString('fr-FR'));
    setText('#map-seed', map.seed);
    setText('#map-resolution', `${map.people.length.toLocaleString('fr-FR')} personnes · ${map.settlements.length} implantations`);
  }

  function updateSimulationStats(simulation) {
    simulationTick = simulation.tick;
    const people = simulation.map?.people || map?.people;
    if (people) {
      pendingSimulationStats = {
        tick: simulationTick,
        infected: people.filter((person) => person.infected && !person.dead).length,
        dead: people.filter((person) => person.dead).length,
        immune: people.filter((person) => person.immune && !person.dead).length,
      };
    } else {
      pendingSimulationStats = {
        tick: simulationTick,
        infected: simulation.board.cells.filter((cell) => cell === 2).length,
        dead: simulation.board.cells.filter((cell) => cell === 3).length,
        immune: simulation.board.cells.filter((cell) => cell === 4).length,
      };
    }
    if (simulationRenderScheduled) return;
    simulationRenderScheduled = true;
    window.requestAnimationFrame(() => {
      simulationRenderScheduled = false;
      if (!pendingSimulationStats) return;
      setText('#simulation-tick', pendingSimulationStats.tick);
      setText('#simulation-infected', pendingSimulationStats.infected);
      setText('#simulation-dead', pendingSimulationStats.dead);
      setText('#simulation-immune', pendingSimulationStats.immune);
    });
  }

  function updateSpeedLabel() {
    const speed = Number(document.querySelector('#speed-slider').value);
    setText('#speed-label', speed === 0 ? 'Sans limite' : `${speed} tour${speed > 1 ? 's' : ''}/s`);
  }

  function hasInfectedPeople() {
    return Boolean(map && map.people.some((person) => person.infected && !person.dead));
  }

  function formatMilliseconds(seconds) {
    return `${(seconds * 1000).toFixed(3)} ms`;
  }

  function updateBenchmark(report) {
    const result = report.results && report.results[0];
    if (!result) return;
    setText('#benchmark-mean', formatMilliseconds(result.meanSeconds));
    const rows = [
      ['Médiane', formatMilliseconds(result.medianSeconds)],
      ['Minimum', formatMilliseconds(result.minSeconds)],
      ['Maximum', formatMilliseconds(result.maxSeconds)],
      ['Écart-type', formatMilliseconds(result.stddevSeconds)],
    ];
    document.querySelector('#benchmark-table').innerHTML = rows.map(([label, value]) => `<tr><td>${label}</td><td>${value}</td></tr>`).join('');
    setText('#benchmark-meta', `${report.configuration.runs} runs · ${report.configuration.warmup} warmup`);
    document.querySelector('#benchmark-empty').classList.add('hidden');
    document.querySelector('#benchmark-content').classList.remove('hidden');
  }

  async function loadSimulation() {
    const response = await fetch('/api/simulation');
    if (!response.ok) throw new Error(`simulation HTTP ${response.status}`);
    const simulation = await response.json();
    if (simulation.map) {
      map = simulation.map;
      updateStats();
      draw();
    }
    updateSimulationStats(simulation);
    updateRuleInputs(simulation);
  }

  async function loadBenchmarks() {
    const response = await fetch('/api/benchmarks');
    if (!response.ok) return;
    updateBenchmark(await response.json());
  }

  async function advanceSimulation() {
    if (tickInFlight) return;
    tickInFlight = true;
    try {
      const response = await fetch('/api/tick', { method: 'POST' });
      if (!response.ok) throw new Error(`tick HTTP ${response.status}`);
      const board = await response.json();
      if (board.people) {
        map = board;
        updateStats();
        draw();
        await loadSimulation();
      } else {
        updateSimulationStats({ tick: simulationTick + 1, board });
      }
    } finally {
      tickInFlight = false;
    }
  }

  async function resetSimulation() {
    pauseSimulation();
    const response = await fetch('/api/reset', { method: 'POST' });
    if (!response.ok) throw new Error(`reset HTTP ${response.status}`);
    const board = await response.json();
    if (board.people) {
      map = board;
      updateStats();
      draw();
      await loadSimulation();
    } else {
      updateSimulationStats({ tick: 0, board });
    }
  }

  function updateRuleInputs(simulation) {
    const values = {
      '#seed-input': simulation.seed,
      '#close-radius-input': simulation.rules.closeRadius,
      '#far-radius-input': simulation.rules.farRadius,
      '#close-chance-input': simulation.rules.closeChance,
      '#far-chance-input': simulation.rules.farChance,
      '#death-chance-input': simulation.rules.deathChance,
      '#recovery-chance-input': simulation.rules.recoveryChance,
      '#immunity-chance-input': simulation.rules.immunityChance,
    };
    for (const [selector, value] of Object.entries(values)) setTextValue(selector, value);
  }

  function setTextValue(selector, value) {
    document.querySelector(selector).value = value;
  }

  function readSetting(selector) {
    return Number(document.querySelector(selector).value);
  }

  async function applySettings() {
    pauseSimulation();
    const feedback = document.querySelector('#settings-feedback');
    const button = document.querySelector('#apply-settings');
    button.disabled = true;
    feedback.textContent = 'Application…';
    try {
      const rules = {
        closeRadius: readSetting('#close-radius-input'),
        closeChance: readSetting('#close-chance-input'),
        farRadius: readSetting('#far-radius-input'),
        farChance: readSetting('#far-chance-input'),
        deathChance: readSetting('#death-chance-input'),
        recoveryChance: readSetting('#recovery-chance-input'),
        immunityChance: readSetting('#immunity-chance-input'),
      };
      const rulesResponse = await fetch('/api/rules', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(rules),
      });
      if (!rulesResponse.ok) throw new Error(await rulesResponse.text());
      const resetResponse = await fetch('/api/reset', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ seed: readSetting('#seed-input') }),
      });
      if (!resetResponse.ok) throw new Error(await resetResponse.text());
      map = await resetResponse.json();
      updateStats();
      draw();
      await loadSimulation();
      feedback.textContent = 'Nouvelle partie prête';
    } catch (error) {
      feedback.textContent = `Erreur : ${error.message}`;
    } finally {
      button.disabled = false;
    }
  }

  function wait(milliseconds) {
    return new Promise((resolve) => window.setTimeout(resolve, milliseconds));
  }

  async function runSimulation(token) {
    let nextTickAt = performance.now();
    while (simulationRunning && token === runToken) {
      try {
        await advanceSimulation();
      } catch (error) {
        console.error(error);
        pauseSimulation();
        return;
      }
      if (!hasInfectedPeople()) {
        pauseSimulation();
        status.dataset.state = 'ready';
        statusLabel.textContent = 'Propagation terminée';
        return;
      }
      const speed = Number(document.querySelector('#speed-slider').value);
      if (speed === 0) continue;
      nextTickAt += 1000 / speed;
      const remainingDelay = nextTickAt - performance.now();
      if (remainingDelay > 0) await wait(remainingDelay);
      if (remainingDelay <= 0) nextTickAt = performance.now();
    }
  }

  function startSimulation() {
    if (simulationRunning) return;
    simulationRunning = true;
    runToken += 1;
    document.querySelector('#start-button').disabled = true;
    document.querySelector('#pause-button').disabled = false;
    runSimulation(runToken);
  }

  function pauseSimulation() {
    simulationRunning = false;
    runToken += 1;
    document.querySelector('#start-button').disabled = false;
    document.querySelector('#pause-button').disabled = true;
  }

  function mapPoint(event) {
    const bounds = canvas.getBoundingClientRect();
    return { x: (event.clientX - bounds.left - view.offsetX) / view.scale, y: (event.clientY - bounds.top - view.offsetY) / view.scale };
  }

  function zoomAt(factor, clientX, clientY) {
    if (!map) return;
    const bounds = canvas.getBoundingClientRect();
    const x = clientX - bounds.left;
    const y = clientY - bounds.top;
    const mapX = (x - view.offsetX) / view.scale;
    const mapY = (y - view.offsetY) / view.scale;
    view.scale = Math.max(0.03, Math.min(8, view.scale * factor));
    view.offsetX = x - mapX * view.scale;
    view.offsetY = y - mapY * view.scale;
    draw();
  }

  canvas.addEventListener('pointerdown', (event) => { view.dragging = true; view.pointerX = event.clientX; view.pointerY = event.clientY; canvas.setPointerCapture(event.pointerId); });
  canvas.addEventListener('pointermove', (event) => {
    const point = mapPoint(event);
    coordinates.textContent = `Position : ${Math.round(point.x)}, ${Math.round(point.y)}`;
    if (!view.dragging) return;
    view.offsetX += event.clientX - view.pointerX;
    view.offsetY += event.clientY - view.pointerY;
    view.pointerX = event.clientX; view.pointerY = event.clientY;
    draw();
  });
  canvas.addEventListener('pointerup', () => { view.dragging = false; });
  canvas.addEventListener('pointercancel', () => { view.dragging = false; });
  canvas.addEventListener('wheel', (event) => { event.preventDefault(); zoomAt(event.deltaY < 0 ? 1.12 : 0.89, event.clientX, event.clientY); }, { passive: false });
  document.querySelector('#fit-button').addEventListener('click', fitMap);
  document.querySelector('#zoom-in').addEventListener('click', () => { const bounds = canvas.getBoundingClientRect(); zoomAt(1.25, bounds.left + bounds.width / 2, bounds.top + bounds.height / 2); });
  document.querySelector('#zoom-out').addEventListener('click', () => { const bounds = canvas.getBoundingClientRect(); zoomAt(0.8, bounds.left + bounds.width / 2, bounds.top + bounds.height / 2); });
  document.querySelector('#start-button').addEventListener('click', startSimulation);
  document.querySelector('#pause-button').addEventListener('click', pauseSimulation);
  document.querySelector('#reset-button').addEventListener('click', () => resetSimulation().catch(console.error));
  document.querySelector('#apply-settings').addEventListener('click', () => applySettings().catch(console.error));
  document.querySelector('#speed-slider').addEventListener('input', updateSpeedLabel);
  window.addEventListener('resize', resizeCanvas);

  async function loadMap() {
    try {
      const response = await fetch('/api/map');
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      map = await response.json();
      updateStats();
      status.dataset.state = 'ready';
      statusLabel.textContent = 'Carte synchronisée';
      loadingCard.classList.add('hidden');
      resizeCanvas();
      fitMap();
      await Promise.all([loadSimulation(), loadBenchmarks()]);
    } catch (error) {
      console.error(error);
      status.dataset.state = 'error';
      statusLabel.textContent = 'Lecture impossible';
      loadingMessage.textContent = 'La carte est indisponible. Vérifiez que le serveur Go est lancé.';
      loadingCard.querySelector('.loader').style.display = 'none';
    }
  }

  loadMap();
  updateSpeedLabel();
})();
