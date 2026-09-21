const canvas = document.querySelector("#board");
const context = canvas.getContext("2d");
const startButton = document.querySelector("#start");
const pauseButton = document.querySelector("#pause");
const resetButton = document.querySelector("#reset");
const generationElement = document.querySelector("#generation");
const healthyCountElement = document.querySelector("#healthy-count");
const infectedCountElement = document.querySelector("#infected-count");
const placementModeElement = document.querySelector("#placement-mode");
const closeRadiusInput = document.querySelector("#close-radius");
const closeChanceInput = document.querySelector("#close-chance");
const farRadiusInput = document.querySelector("#far-radius");
const farChanceInput = document.querySelector("#far-chance");
const populationInput = document.querySelector("#population");
const initialInfectedInput = document.querySelector("#initial-infected");

const worldSize = 400;
const initialViewSize = 100;
const minCellSize = 0.035;
const maxCellSize = 80;
const maxPopulation = worldSize * worldSize;
let cellSize = Math.max(4, Math.min(maxCellSize, window.innerWidth / initialViewSize));
let cameraX = worldSize / 2;
let cameraY = worldSize / 2;
let generation = 0;
let running = false;
let animationFrame;
let dragStart;
const healthyCells = new Set();
const infectedCells = new Set();

function clamp(value, minimum, maximum) {
  return Math.max(minimum, Math.min(maximum, value));
}

function randomInt(maximum) {
  return Math.floor(Math.random() * maximum);
}

function populationLimit() {
  return clamp(Number(populationInput.value) || 0, 1, maxPopulation);
}

function infectedLimit(targetPopulation) {
  return clamp(Number(initialInfectedInput.value) || 0, 1, targetPopulation);
}

function contaminationRules() {
  return {
    closeRadius: Math.max(0, Number(closeRadiusInput.value) || 0),
    closeChance: clamp((Number(closeChanceInput.value) || 0) / 100, 0, 1),
    farRadius: Math.max(0, Number(farRadiusInput.value) || 0),
    farChance: clamp((Number(farChanceInput.value) || 0) / 100, 0, 1),
  };
}

function formatCount(count) {
  return new Intl.NumberFormat("fr-FR").format(count);
}

function updateStats() {
  generationElement.textContent = `Génération : ${generation}`;
  healthyCountElement.textContent = `Sains : ${formatCount(healthyCells.size)}`;
  infectedCountElement.textContent = `Contaminés : ${formatCount(infectedCells.size)}`;
}

function resizeCanvas() {
  const pixelRatio = window.devicePixelRatio || 1;
  canvas.width = Math.floor(canvas.clientWidth * pixelRatio);
  canvas.height = Math.floor(canvas.clientHeight * pixelRatio);
  context.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0);
  render();
}

function worldToScreen(column, row) {
  return {
    x: (column - cameraX) * cellSize + canvas.clientWidth / 2,
    y: (row - cameraY) * cellSize + canvas.clientHeight / 2,
  };
}

function screenToWorld(x, y) {
  return {
    column: Math.floor((x - canvas.clientWidth / 2) / cellSize + cameraX),
    row: Math.floor((y - canvas.clientHeight / 2) / cellSize + cameraY),
  };
}

function cellKey(column, row) {
  return `${column},${row}`;
}

function parseCellKey(key) {
  const [column, row] = key.split(",").map(Number);
  return { column, row };
}

function isInsideWorld(column, row) {
  return column >= 0 && column < worldSize && row >= 0 && row < worldSize;
}

function renderGrid() {
  const maxLines = 250;
  const visibleColumns = canvas.clientWidth / cellSize;
  const visibleRows = canvas.clientHeight / cellSize;
  const interval = Math.max(1, Math.ceil(Math.max(visibleColumns, visibleRows) / maxLines));
  const left = Math.max(0, Math.floor(cameraX - visibleColumns / 2));
  const right = Math.min(worldSize, Math.ceil(cameraX + visibleColumns / 2));
  const top = Math.max(0, Math.floor(cameraY - visibleRows / 2));
  const bottom = Math.min(worldSize, Math.ceil(cameraY + visibleRows / 2));

  context.beginPath();
  context.strokeStyle = "rgba(255, 255, 255, 0.08)";
  context.lineWidth = 1;
  for (let column = left - (left % interval); column <= right; column += interval) {
    const x = worldToScreen(column, 0).x + 0.5;
    context.moveTo(x, 0);
    context.lineTo(x, canvas.clientHeight);
  }
  for (let row = top - (top % interval); row <= bottom; row += interval) {
    const y = worldToScreen(0, row).y + 0.5;
    context.moveTo(0, y);
    context.lineTo(canvas.clientWidth, y);
  }
  context.stroke();
}

function renderCells(cells, color) {
  context.fillStyle = color;
  for (const key of cells) {
    const { column, row } = parseCellKey(key);
    const position = worldToScreen(column, row);
    if (position.x + cellSize < 0 || position.y + cellSize < 0 || position.x > canvas.clientWidth || position.y > canvas.clientHeight) continue;
    context.fillRect(position.x, position.y, Math.max(1, cellSize), Math.max(1, cellSize));
  }
}

function render() {
  context.fillStyle = "#030303";
  context.fillRect(0, 0, canvas.clientWidth, canvas.clientHeight);
  renderGrid();
  renderCells(healthyCells, "#f2f2f2");
  renderCells(infectedCells, "#ff5d5d");
}

function clearPopulation() {
  healthyCells.clear();
  infectedCells.clear();
}

function placeCell(column, row, mode) {
  if (!isInsideWorld(column, row)) return;
  const key = cellKey(column, row);
  if (mode === "infected") {
    if (infectedCells.has(key)) {
      infectedCells.delete(key);
      return;
    }
    healthyCells.delete(key);
    infectedCells.add(key);
    return;
  }
  if (healthyCells.has(key)) {
    healthyCells.delete(key);
    return;
  }
  infectedCells.delete(key);
  healthyCells.add(key);
}

function populateWorld() {
  clearPopulation();
  generation = 0;
  const targetPopulation = populationLimit();
  const targetInfected = infectedLimit(targetPopulation);

  while (healthyCells.size + infectedCells.size < targetPopulation) {
    const column = randomInt(worldSize);
    const row = randomInt(worldSize);
    const key = cellKey(column, row);
    if (healthyCells.has(key) || infectedCells.has(key)) continue;
    healthyCells.add(key);
  }

  const healthyKeys = Array.from(healthyCells);
  for (let index = 0; index < targetInfected && healthyKeys.length > 0; index += 1) {
    const infectedIndex = randomInt(healthyKeys.length);
    const infectedKey = healthyKeys.splice(infectedIndex, 1)[0];
    healthyCells.delete(infectedKey);
    infectedCells.add(infectedKey);
  }

  updateStats();
  render();
}

function shouldInfect(distanceSquared, rules) {
  if (distanceSquared <= rules.closeRadius * rules.closeRadius) {
    return Math.random() < rules.closeChance;
  }
  if (distanceSquared <= rules.farRadius * rules.farRadius) {
    return Math.random() < rules.farChance;
  }
  return false;
}

function nextGeneration() {
  const rules = contaminationRules();
  const closeCandidates = new Set();
  const farCandidates = new Set();
  const farRadius = Math.max(rules.closeRadius, rules.farRadius);
  const farRadiusSquared = farRadius * farRadius;
  const closeRadiusSquared = rules.closeRadius * rules.closeRadius;

  for (const key of infectedCells) {
    const { column, row } = parseCellKey(key);
    for (let rowOffset = -farRadius; rowOffset <= farRadius; rowOffset += 1) {
      const targetRow = row + rowOffset;
      if (targetRow < 0 || targetRow >= worldSize) continue;
      for (let columnOffset = -farRadius; columnOffset <= farRadius; columnOffset += 1) {
        const targetColumn = column + columnOffset;
        if (targetColumn < 0 || targetColumn >= worldSize) continue;
        const distanceSquared = columnOffset * columnOffset + rowOffset * rowOffset;
        if (distanceSquared === 0 || distanceSquared > farRadiusSquared) continue;
        const targetKey = cellKey(targetColumn, targetRow);
        if (!healthyCells.has(targetKey)) continue;
        if (distanceSquared <= closeRadiusSquared) {
          closeCandidates.add(targetKey);
        } else {
          farCandidates.add(targetKey);
        }
      }
    }
  }

  for (const key of closeCandidates) {
    if (!healthyCells.has(key)) continue;
    if (Math.random() < rules.closeChance) {
      healthyCells.delete(key);
      infectedCells.add(key);
    }
  }

  for (const key of farCandidates) {
    if (!healthyCells.has(key) || infectedCells.has(key)) continue;
    if (Math.random() < rules.farChance) {
      healthyCells.delete(key);
      infectedCells.add(key);
    }
  }

  generation += 1;
  updateStats();
  render();
}

function animate() {
  if (!running) return;
  nextGeneration();
  animationFrame = requestAnimationFrame(animate);
}

canvas.addEventListener("pointerdown", (event) => {
  canvas.setPointerCapture(event.pointerId);
  dragStart = { x: event.clientX, y: event.clientY, cameraX, cameraY };
});

canvas.addEventListener("pointerup", (event) => {
  if (!dragStart) return;
  const moved = Math.hypot(event.clientX - dragStart.x, event.clientY - dragStart.y);
  if (!running && moved < 5) {
    const rectangle = canvas.getBoundingClientRect();
    const position = screenToWorld(event.clientX - rectangle.left, event.clientY - rectangle.top);
    placeCell(position.column, position.row, placementModeElement.value);
    updateStats();
    render();
  }
  dragStart = null;
});

canvas.addEventListener("pointermove", (event) => {
  if (!dragStart) return;
  cameraX = clamp(dragStart.cameraX - (event.clientX - dragStart.x) / cellSize, 0, worldSize - 1);
  cameraY = clamp(dragStart.cameraY - (event.clientY - dragStart.y) / cellSize, 0, worldSize - 1);
  render();
});

canvas.addEventListener("wheel", (event) => {
  event.preventDefault();
  const rectangle = canvas.getBoundingClientRect();
  const before = screenToWorld(event.clientX - rectangle.left, event.clientY - rectangle.top);
  cellSize = clamp(cellSize * (event.deltaY < 0 ? 1.2 : 1 / 1.2), minCellSize, maxCellSize);
  const after = screenToWorld(event.clientX - rectangle.left, event.clientY - rectangle.top);
  cameraX = clamp(cameraX + before.column - after.column, 0, worldSize - 1);
  cameraY = clamp(cameraY + before.row - after.row, 0, worldSize - 1);
  render();
}, { passive: false });

startButton.addEventListener("click", () => {
  if (running || infectedCells.size === 0) return;
  running = true;
  animationFrame = requestAnimationFrame(animate);
});

pauseButton.addEventListener("click", () => {
  running = false;
  cancelAnimationFrame(animationFrame);
});

resetButton.addEventListener("click", () => {
  running = false;
  cancelAnimationFrame(animationFrame);
  populateWorld();
});

window.addEventListener("resize", resizeCanvas);
updateStats();
populateWorld();
resizeCanvas();
